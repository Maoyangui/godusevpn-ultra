// Read PE manifests and VERSIONINFO; never execute the product.
'use strict';
const fs = require('node:fs'), path = require('node:path'), crypto = require('node:crypto'), assert = require('node:assert/strict');

function numericVersion(version) {
  const match = /^(\d+)\.(\d+)(?:\.(\d+))?(?:-[A-Za-z0-9.-]+)?$/.exec(version);
  if (!match) throw Error('Unsupported descriptor version');
  const parts = [match[1], match[2], match[3] || '0', '0'].map(Number);
  if (parts.some(n => !Number.isSafeInteger(n) || n < 0 || n > 65535)) throw Error('PE version component out of range');
  return parts.join('.');
}

function readPEResources(b, expectedMachine) {
  const need = (off, len) => { if (!Number.isSafeInteger(off) || off < 0 || len < 0 || off + len > b.length) throw Error('Truncated PE image'); };
  need(0, 64);
  if (b.readUInt16LE(0) !== 0x5a4d) throw Error('Not MZ');
  const pe = b.readUInt32LE(0x3c); need(pe, 24);
  if (b.toString('ascii', pe, pe + 4) !== 'PE\0\0' || b.readUInt16LE(pe + 4) !== expectedMachine) throw Error('Unexpected PE architecture');
  const optional = pe + 24, optionalSize = b.readUInt16LE(pe + 20), count = b.readUInt16LE(pe + 6);
  need(optional, optionalSize + count * 40);
  if (optionalSize < 136 || b.readUInt16LE(optional) !== 0x20b || count < 1 || count > 96 || b.readUInt32LE(optional + 108) < 3) throw Error('Expected a bounded PE32+ image');
  const sections = [];
  for (let i = 0; i < count; i++) {
    const off = optional + optionalSize + i * 40;
    sections.push({ rva: b.readUInt32LE(off + 12), size: b.readUInt32LE(off + 16), raw: b.readUInt32LE(off + 20) });
  }
  const offset = (rva, bytes) => {
    const matches = sections.filter(s => rva >= s.rva && rva - s.rva + bytes <= s.size);
    if (matches.length !== 1) throw Error('Resource RVA is ambiguous/outside raw sections');
    const result = matches[0].raw + rva - matches[0].rva; need(result, bytes); return result;
  };
  const rva = b.readUInt32LE(optional + 128), size = b.readUInt32LE(optional + 132);
  if (!size || size > 32 << 20) throw Error('Resource section limit');
  const base = offset(rva, size), end = base + size;
  const bounded = (rel, bytes) => {
    if (rel < 0 || base + rel + bytes > end) throw Error('Resource directory is outside bounds');
    return base + rel;
  };
  const entries = rel => {
    const off = bounded(rel, 16), n = b.readUInt16LE(off + 12) + b.readUInt16LE(off + 14);
    if (n > 4096) throw Error('Too many resources');
    bounded(rel + 16, n * 8);
    return Array.from({ length: n }, (_, i) => ({ id: b.readUInt32LE(off + 16 + i * 8), target: b.readUInt32LE(off + 20 + i * 8) }));
  };
  const result = new Map();
  for (const type of entries(0)) {
    if (type.id !== 16 && type.id !== 24) continue;
    const values = [], active = new Set();
    const visit = (entry, depth) => {
      if (depth > 3) throw Error('Deep resource tree');
      if (entry.target & 0x80000000) {
        const relative = entry.target & 0x7fffffff;
        if (active.has(relative)) throw Error('Cyclic resource tree');
        active.add(relative);
        for (const child of entries(relative)) visit(child, depth + 1);
        active.delete(relative);
      } else {
        const off = bounded(entry.target, 16), len = b.readUInt32LE(off + 4);
        if (len > 1 << 20) throw Error('Manifest/version resource too large');
        const start = offset(b.readUInt32LE(off), len);
        values.push(b.subarray(start, start + len));
      }
    };
    visit(type, 0);
    if (result.has(type.id)) throw Error('Duplicate resource type');
    result.set(type.id, values);
  }
  return result;
}

function readVersionResource(b) {
  const align = n => Math.ceil(n / 4) * 4;
  let nodes = 0;
  function node(start, parentEnd, depth) {
    if (depth > 6 || ++nodes > 4096 || start + 6 > parentEnd) throw Error('Invalid VERSIONINFO tree');
    const length = b.readUInt16LE(start), valueLength = b.readUInt16LE(start + 2), type = b.readUInt16LE(start + 4), end = start + length;
    if (length < 8 || end > parentEnd || type > 1) throw Error('Invalid VERSIONINFO bounds');
    let off = start + 6, keyEnd = -1;
    while (off + 2 <= end) { if (b.readUInt16LE(off) === 0) { keyEnd = off; break; } off += 2; }
    if (keyEnd < 0) throw Error('Unterminated VERSIONINFO key');
    const key = b.toString('utf16le', start + 6, keyEnd), valueStart = align(keyEnd + 2), valueBytes = type === 1 ? valueLength * 2 : valueLength;
    if (valueBytes && valueStart + valueBytes > end) throw Error('Invalid VERSIONINFO value');
    const value = valueBytes ? b.subarray(valueStart, valueStart + valueBytes) : Buffer.alloc(0), children = [];
    off = align(valueStart + valueBytes);
    while (off + 6 <= end) {
      if (b.readUInt16LE(off) === 0) break;
      const child = node(off, end, depth + 1); children.push(child); off = align(child.end);
    }
    return { key, value, type, children, end };
  }
  const root = node(0, b.length, 0);
  if (root.key !== 'VS_VERSION_INFO' || root.type !== 0 || root.value.length !== 52 || root.value.readUInt32LE(0) !== 0xfeef04bd) throw Error('Missing VS_FIXEDFILEINFO');
  const words = (ms, ls) => [ms >>> 16, ms & 65535, ls >>> 16, ls & 65535].join('.');
  const fixedFileVersion = words(root.value.readUInt32LE(8), root.value.readUInt32LE(12));
  const fixedProductVersion = words(root.value.readUInt32LE(16), root.value.readUInt32LE(20));
  const strings = [];
  for (const info of root.children.filter(n => n.key === 'StringFileInfo')) {
    for (const table of info.children) {
      if (!/^[0-9a-f]{8}$/i.test(table.key)) throw Error('Invalid version string table');
      const values = {};
      for (const item of table.children) {
        if (item.type !== 1 || Object.hasOwn(values, item.key)) throw Error('Invalid/duplicate version string');
        values[item.key] = item.value.toString('utf16le').replace(/\0+$/, '');
      }
      strings.push(values);
    }
  }
  if (!strings.length) throw Error('Missing version string table');
  return { fixedFileVersion, fixedProductVersion, strings };
}

function manifestText(data) {
  if (data[0] === 0xfe && data[1] === 0xff) throw Error('Unsupported manifest encoding');
  return data.toString(data[1] === 0 || data[0] === 0xff && data[1] === 0xfe ? 'utf16le' : 'utf8').replace(/^\uFEFF/, '');
}

function verifyLocationConsumer(root, descriptor) {
  const directory = path.join(root, 'location-consumer');
  if (!Object.hasOwn(descriptor, 'locationConsumer')) {
    if (fs.existsSync(directory)) throw Error('Undeclared location consumer payload');
    return null;
  }
  const consumer = descriptor.locationConsumer;
  if (!consumer || descriptor.architecture !== 'amd64' || consumer.schemaVersion !== 1 || consumer.implementation !== 'winrt-geolocator' || consumer.architecture !== 'amd64' || consumer.minimumFramework !== '4.6.2' || consumer.nativeRuntimeBundled !== false || consumer.vmValidated !== false) throw Error('Unsupported location consumer declaration');
  const required = ['GodUseVPN.Ultra.LocationConsumer.exe', 'GodUseVPN.Ultra.LocationConsumer.exe.config'];
  if (!fs.lstatSync(directory).isDirectory() || fs.lstatSync(directory).isSymbolicLink()) throw Error('Invalid location consumer directory');
  assert.deepEqual(fs.readdirSync(directory).sort(), required.slice().sort(), 'Unexpected location consumer payload');
  const files = required.map(name => {
    const file = path.join(directory, name), stat = fs.lstatSync(file);
    if (!stat.isFile() || stat.isSymbolicLink() || stat.size === 0 || stat.size > 1 << 20) throw Error('Invalid location consumer payload');
    const data = fs.readFileSync(file);
    return { name:'location-consumer/' + name, bytes:data.length, sha256:crypto.createHash('sha256').update(data).digest('hex') };
  });
  const resources = readPEResources(fs.readFileSync(path.join(directory, required[0])), 0x8664), manifests = resources.get(24) || [];
  if (manifests.length !== 1) throw Error('Location consumer needs exactly one manifest');
  const manifest = manifestText(manifests[0]), levels = manifest.match(/<requestedExecutionLevel\b[^>]*>/g) || [], identities = manifest.match(/<assemblyIdentity\b[^>]*>/g) || [];
  if (levels.length !== 1 || !/\blevel="asInvoker"/.test(levels[0]) || !/\buiAccess="false"/.test(levels[0]) || identities.length !== 1 || !/\bname="GodUseVPN\.Ultra\.LocationConsumer"/.test(identities[0]) || !/\bversion="1\.0\.0\.0"/.test(identities[0])) throw Error('Location consumer privilege/identity mismatch');
  const config = '<?xml version="1.0"?><configuration><startup><supportedRuntime version="v4.0" sku=".NETFramework,Version=v4.6.2"/></startup></configuration>';
  if (!fs.readFileSync(path.join(directory, required[1])).equals(Buffer.from(config, 'utf8'))) throw Error('Location consumer runtime configuration mismatch');
  return {...consumer, executionLevel:'asInvoker', runtimeVerified:false, files};
}

function verifyPackageResources(root) {
  if (fs.statSync(path.join(root, 'ULTRA_PACKAGE.json')).size > 64 << 10) throw Error('Package descriptor too large');
  const descriptor = JSON.parse(fs.readFileSync(path.join(root, 'ULTRA_PACKAGE.json'), 'utf8').replace(/^\uFEFF/, ''));
  if (descriptor.namespace !== 'godusevpn-ultra' || descriptor.product !== 'GodUseVPN Ultra' || descriptor.unsigned !== true) throw Error('Not an unsigned Ultra package');
  const expectedMachine = { amd64: 0x8664, arm64: 0xaa64 }[descriptor.architecture];
  if (!expectedMachine) throw Error('Unsupported package architecture');
  const expectedNumericVersion = numericVersion(descriptor.version);
  const binaries = ['godusevpn.exe', 'godusevpn-svc.exe', 'godusevpn-cli.exe'].map(name => {
    const b = fs.readFileSync(path.join(root, name)), resources = readPEResources(b, expectedMachine);
    const manifests = resources.get(24) || [], versions = resources.get(16) || [];
    if (manifests.length !== 1 || versions.length !== 1) throw Error('Expected exactly one RT_MANIFEST and RT_VERSION: ' + name);
    const xml = manifestText(manifests[0]), requested = xml.match(/<requestedExecutionLevel\b[^>]*>/g) || [];
    if (requested.length !== 1 || !/\blevel="asInvoker"/.test(requested[0]) || !/\buiAccess="false"/.test(requested[0])) throw Error('Unexpected execution privilege: ' + name);
    const identity = name.replace('godusevpn', 'godusevpn-ultra').replace(/\.exe$/, '');
    const identities = (xml.match(/<assemblyIdentity\b[^>]*>/g) || []).filter(s => s.includes('name="' + identity + '"'));
    if (identities.length !== 1 || !identities[0].includes('version="' + expectedNumericVersion + '"')) throw Error('Unexpected assembly identity/version: ' + name);
    const version = readVersionResource(versions[0]);
    assert.equal(version.fixedFileVersion, expectedNumericVersion, 'Fixed FileVersion mismatch: ' + name);
    assert.equal(version.fixedProductVersion, expectedNumericVersion, 'Fixed ProductVersion mismatch: ' + name);
    for (const table of version.strings) {
      assert.equal(table.ProductVersion, descriptor.version, 'ProductVersion string mismatch: ' + name);
      if (!/^\d+\.\d+(?:\.\d+){0,2}$/.test(table.FileVersion || '')) throw Error('Missing numeric FileVersion string: ' + name);
      const parts = table.FileVersion.split('.').map(Number); while (parts.length < 4) parts.push(0);
      assert.equal(parts.join('.'), expectedNumericVersion, 'FileVersion string mismatch: ' + name);
    }
    return { name, bytes: b.length, sha256: crypto.createHash('sha256').update(b).digest('hex'), machine: descriptor.architecture, executionLevel: 'asInvoker', assemblyIdentity: identity, fixedFileVersion: version.fixedFileVersion, fixedProductVersion: version.fixedProductVersion, productVersion: descriptor.version };
  });
  let browserHost;
  if (Object.hasOwn(descriptor, 'browserHost')) {
    const host = descriptor.browserHost;
    if (!host || descriptor.architecture !== 'amd64' || host.schemaVersion !== 1 || host.implementation !== 'webview2-winforms' || host.architecture !== 'amd64' || host.sdkVersion !== '1.0.4314.35' || host.nativeRuntimeBundled !== false) throw Error('Unsupported browser host declaration');
    const required = ['GodUseVPN.Ultra.BrowserHost.exe', 'GodUseVPN.Ultra.BrowserHost.exe.config', 'Microsoft.Web.WebView2.Core.dll', 'Microsoft.Web.WebView2.WinForms.dll', 'WebView2Loader.dll', 'Microsoft.Web.WebView2.LICENSE.txt', 'README.md'];
    const files = required.map(name => {
      const file = path.join(root, 'browser-host', name);
      if (fs.lstatSync(file).isSymbolicLink() || !fs.statSync(file).isFile()) throw Error('Invalid browser host payload');
      const bytes = fs.readFileSync(file);
      if (!bytes.length) throw Error('Empty browser host payload');
      return { name: 'browser-host/' + name, bytes: bytes.length, sha256: crypto.createHash('sha256').update(bytes).digest('hex') };
    });
    const resources = readPEResources(fs.readFileSync(path.join(root, files[0].name)), 0x8664);
    const manifests = resources.get(24) || [], versions = resources.get(16) || [];
    if (manifests.length !== 1 || versions.length !== 1) throw Error('Browser host needs one manifest and VERSIONINFO');
    const manifest = manifestText(manifests[0]), levels = manifest.match(/<requestedExecutionLevel\b[^>]*>/g) || [];
    if (levels.length !== 1 || !/\blevel="asInvoker"/.test(levels[0]) || !/\buiAccess="false"/.test(levels[0]) || !manifest.includes('name="GodUseVPN.Ultra.BrowserHost"')) throw Error('Browser host privilege/identity mismatch');
    const version = readVersionResource(versions[0]);
    assert.equal(version.fixedFileVersion, expectedNumericVersion, 'Browser host file version mismatch');
    for (const table of version.strings) assert.equal(table.ProductVersion, descriptor.version, 'Browser host product version mismatch');
    browserHost = { ...host, executionLevel: 'asInvoker', version: descriptor.version, runtimeVerified: false, files };
  }
  let metadataTool;
  if (Object.hasOwn(descriptor, 'metadataTool')) {
    const expected = descriptor.metadataTool, fixedManifest = 'd1316fddf986e2d17761f85034c753b28375f25bf73235c1084a7c5a38e22faf';
    if (!expected || descriptor.architecture !== 'amd64' || expected.version !== '13.59' || expected.implementation !== 'exiftool' || expected.manifestSHA256 !== fixedManifest || expected.fileCount !== 1501 || expected.vmValidated !== false) throw Error('Unsupported metadata tool declaration');
    const manifest = fs.readFileSync(path.join(root, 'metadata-tool/exiftool-13.59.sha256'));
    if (crypto.createHash('sha256').update(manifest).digest('hex') !== fixedManifest) throw Error('Pinned metadata manifest mismatch');
    const lines = manifest.toString('utf8').trimEnd().split(/\r?\n/), seen = new Set(); let bytes = 0;
    for (const line of lines) {
      const match = /^([a-f0-9]{64})  (.+)$/.exec(line);
      if (!match || /[\\:\x00-\x1f]/.test(match[2]) || match[2].split('/').some(piece=>!piece || piece==='.' || piece==='..' || /[. ]$/.test(piece)) || seen.has(match[2].toLowerCase())) throw Error('Unsafe metadata manifest member');
      seen.add(match[2].toLowerCase());
      const file = path.join(root, 'metadata-tool/exiftool-13.59', match[2]);
      if (!fs.lstatSync(file).isFile() || fs.lstatSync(file).isSymbolicLink()) throw Error('Invalid metadata bundle member');
      const data = fs.readFileSync(file); bytes += data.length;
      if (crypto.createHash('sha256').update(data).digest('hex') !== match[1]) throw Error('Metadata member hash mismatch');
    }
    if (seen.size !== 1501) throw Error('Incomplete metadata bundle');
    let actualFiles = 0;
    const visit = directory => { for (const entry of fs.readdirSync(directory, {withFileTypes:true})) { const file = path.join(directory, entry.name); if (entry.isSymbolicLink()) throw Error('Metadata bundle link rejected'); if (entry.isDirectory()) visit(file); else if (entry.isFile()) actualFiles++; else throw Error('Metadata bundle special file rejected'); } };
    visit(path.join(root, 'metadata-tool/exiftool-13.59'));
    if (actualFiles !== seen.size) throw Error('Unlisted metadata bundle members');
    metadataTool = {...expected, bytes, runtimeVerified:false};
  }
  const locationConsumer = verifyLocationConsumer(root, descriptor);
  return { version: descriptor.version, numericVersion: expectedNumericVersion, architecture: descriptor.architecture, verification: 'read-only PE manifests, VERSIONINFO and binary hashes; runtime version was not executed or independently verified', runtimeVersionVerified: false, unsignedDeclared: true, binaries, ...(browserHost ? { browserHost } : {}), ...(metadataTool ? {metadataTool} : {}), ...(locationConsumer ? {locationConsumer} : {}) };
}

module.exports = { numericVersion, readPEResources, readVersionResource, verifyPackageResources, verifyLocationConsumer };
if (require.main === module) {
  if (!process.argv[2]) throw Error('Pass the package directory.');
  process.stdout.write(JSON.stringify(verifyPackageResources(path.resolve(process.argv[2])), null, 2) + '\n');
}
