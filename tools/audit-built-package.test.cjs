'use strict';
// Synthetic PE/ZIP data only. No product, browser, installer or network runs.
const fs = require('node:fs'), os = require('node:os'), path = require('node:path'), zlib = require('node:zlib'), assert = require('node:assert/strict');
const { numericVersion, verifyPackageResources } = require('./verify-ultra-package.cjs');
const { digest, crc32, readInventory, verifyManifest, verifyZIPAgainstInventory, verifyArchiveSidecar, auditBuiltPackage } = require('./audit-built-package.cjs');
const repository = path.resolve(__dirname, '..');
const tempBase = path.resolve(os.tmpdir()), temp = fs.mkdtempSync(path.join(tempBase, 'ultra-publication-test-'));
let checks = 0;
const align = n => Math.ceil(n / 4) * 4;

function versionNode(key, value, type, children = []) {
  const keyData = Buffer.from(key + '\0', 'utf16le');
  const valueStart = align(6 + keyData.length);
  let size = valueStart + value.length;
  const positions = children.map(child => { size = align(size); const position = size; size += child.length; return position; });
  const b = Buffer.alloc(size);
  b.writeUInt16LE(size, 0); b.writeUInt16LE(type ? value.length / 2 : value.length, 2); b.writeUInt16LE(type, 4);
  keyData.copy(b, 6); value.copy(b, valueStart); children.forEach((child, i) => child.copy(b, positions[i])); return b;
}
function versionInfo(label, options = {}) {
  const fixed = Buffer.alloc(52); fixed.writeUInt32LE(0xfeef04bd, 0); fixed.writeUInt32LE(0x10000, 4);
  const writeVersion = (label, offset) => { const p = label.split('.').map(Number); fixed.writeUInt32LE((p[0] * 65536 + p[1]) >>> 0, offset); fixed.writeUInt32LE((p[2] * 65536 + p[3]) >>> 0, offset + 4); };
  writeVersion(options.fixedFile || numericVersion(label), 8); writeVersion(options.fixedProduct || numericVersion(label), 16);
  const strings = [versionNode('FileVersion', Buffer.from(numericVersion(label) + '\0', 'utf16le'), 1), versionNode('ProductVersion', Buffer.from((options.productString || label) + '\0', 'utf16le'), 1)];
  return versionNode('VS_VERSION_INFO', fixed, 0, [versionNode('StringFileInfo', Buffer.alloc(0), 1, [versionNode('040904B0', Buffer.alloc(0), 1, strings)])]);
}
function peFixture(name, label, options = {}) {
  const identity = name.replace('godusevpn', 'godusevpn-ultra').replace(/\.exe$/, '');
  const manifest = Buffer.from(options.manifest || '<assembly><assemblyIdentity name="' + identity + '" version="' + numericVersion(label) + '"/><trustInfo><requestedExecutionLevel level="asInvoker" uiAccess="false"/></trustInfo></assembly>');
  const items = [{ id: 24, data: manifest }];
  if (!options.missingVersion) items.unshift({ id: 16, data: versionInfo(label, options) });
  let size = 16 + items.length * 8;
  for (const item of items) { item.type = size; size += 24; item.language = size; size += 24; item.entry = size; size += 16; }
  for (const item of items) { size = align(size); item.payload = size; size += item.data.length; }
  const r = Buffer.alloc(size); r.writeUInt16LE(items.length, 14);
  items.forEach((item, i) => {
    r.writeUInt32LE(item.id, 16 + i * 8); r.writeUInt32LE((0x80000000 + item.type) >>> 0, 20 + i * 8);
    r.writeUInt16LE(1, item.type + 14); r.writeUInt32LE(1, item.type + 16); r.writeUInt32LE((0x80000000 + item.language) >>> 0, item.type + 20);
    r.writeUInt16LE(1, item.language + 14); r.writeUInt32LE(0x409, item.language + 16); r.writeUInt32LE(item.entry, item.language + 20);
    r.writeUInt32LE(0x1000 + item.payload, item.entry); r.writeUInt32LE(item.data.length, item.entry + 4); item.data.copy(r, item.payload);
  });
  const rawSize = Math.ceil(r.length / 512) * 512, b = Buffer.alloc(512 + rawSize);
  b.writeUInt16LE(0x5a4d, 0); b.writeUInt32LE(0x80, 0x3c); b.write('PE\0\0', 0x80, 'ascii');
  b.writeUInt16LE(0x8664, 0x84); b.writeUInt16LE(1, 0x86); b.writeUInt16LE(240, 0x94);
  const optional = 0x98; b.writeUInt16LE(0x20b, optional); b.writeUInt32LE(16, optional + 108); b.writeUInt32LE(0x1000, optional + 128); b.writeUInt32LE(r.length, optional + 132);
  const section = optional + 240; b.write('.rsrc', section); b.writeUInt32LE(r.length, section + 8); b.writeUInt32LE(0x1000, section + 12); b.writeUInt32LE(rawSize, section + 16); b.writeUInt32LE(512, section + 20); r.copy(b, 512);
  return b;
}

function zipFixture(entries, comment = '') {
  let position = 0;
  const locals = [], centrals = [];
  for (const entry of entries) {
    const name = Buffer.from(entry.name), localName = Buffer.from(entry.localName || entry.name), data = Buffer.from(entry.data || ''), method = entry.method || 0, flags = entry.flags || 0;
    const compressed = method === 8 ? zlib.deflateRawSync(data) : data, crc = entry.badCRC ? (crc32(data) ^ 1) >>> 0 : crc32(data), size = entry.size ?? data.length;
    const localExtra = entry.localExtra || Buffer.alloc(0), centralExtra = entry.centralExtra || Buffer.alloc(0);
    const local = Buffer.alloc(30 + localName.length + localExtra.length); local.writeUInt32LE(0x04034b50); local.writeUInt16LE(20, 4); local.writeUInt16LE(flags, 6); local.writeUInt16LE(method, 8);
    if (!(flags & 8)) { local.writeUInt32LE(crc, 14); local.writeUInt32LE(compressed.length, 18); local.writeUInt32LE(size, 22); }
    local.writeUInt16LE(localName.length, 26); local.writeUInt16LE(localExtra.length, 28); localName.copy(local, 30); localExtra.copy(local, 30 + localName.length);
    const central = Buffer.alloc(46 + name.length + centralExtra.length); central.writeUInt32LE(0x02014b50); central.writeUInt16LE(20, 4); central.writeUInt16LE(20, 6); central.writeUInt16LE(flags, 8); central.writeUInt16LE(method, 10);
    central.writeUInt32LE(crc, 16); central.writeUInt32LE(compressed.length, 20); central.writeUInt32LE(size, 24); central.writeUInt16LE(name.length, 28); central.writeUInt16LE(centralExtra.length, 30); central.writeUInt32LE(entry.attributes || 0, 38); central.writeUInt32LE(position, 42); name.copy(central, 46); centralExtra.copy(central, 46 + name.length);
    const descriptor = Buffer.alloc(flags & 8 ? 16 : 0);
    if (flags & 8) { descriptor.writeUInt32LE(0x08074b50); descriptor.writeUInt32LE(crc, 4); descriptor.writeUInt32LE(compressed.length, 8); descriptor.writeUInt32LE(size, 12); }
    locals.push(local, compressed, descriptor); centrals.push(central); position += local.length + compressed.length + descriptor.length;
  }
  const central = Buffer.concat(centrals), extra = Buffer.from(comment), end = Buffer.alloc(22); end.writeUInt32LE(0x06054b50); end.writeUInt16LE(entries.length, 8); end.writeUInt16LE(entries.length, 10); end.writeUInt32LE(central.length, 12); end.writeUInt32LE(position, 16); end.writeUInt16LE(extra.length, 20);
  return Buffer.concat([...locals, central, end, extra]);
}
function writePackage(pkg) {
  fs.mkdirSync(path.join(pkg, 'browser-extension'), { recursive: true });
  const label = '0.01-beta';
  for (const name of ['godusevpn.exe', 'godusevpn-svc.exe', 'godusevpn-cli.exe']) {
    let b = peFixture(name, label);
    if (name === 'godusevpn.exe') b = Buffer.concat([b, ...['app.js', 'i18n.js', 'index.html', 'style.css', 'privacy.js', 'privacy.css'].map(n => fs.readFileSync(path.join(repository, 'web/dist', n)))]);
    fs.writeFileSync(path.join(pkg, name), b);
  }
  fs.writeFileSync(path.join(pkg, 'ULTRA_PACKAGE.json'), JSON.stringify({ product: 'GodUseVPN Ultra', namespace: 'godusevpn-ultra', unsigned: true, architecture: 'amd64', version: label }));
  for (const name of ['Install-Ultra.ps1', 'Uninstall-Ultra.ps1', 'browser-extension/manifest.json']) fs.writeFileSync(path.join(pkg, name), 'synthetic fixture');
  const inventory = readInventory(pkg);
  fs.writeFileSync(path.join(pkg, 'SHA256SUMS'), [...inventory.files].map(([name, info]) => info.sha256 + '  ' + name).join('\n') + '\n');
}
function entriesFor(pkg, inventory) { return [...inventory.files].map(([name]) => ({ name: path.basename(pkg) + '/' + name, data: fs.readFileSync(path.join(pkg, name)) })); }
function rejects(fn, label) { assert.throws(fn, undefined, label); checks++; }

try {
  assert.equal(numericVersion('0.01-beta'), '0.1.0.0'); checks++;
  const pkg = path.join(temp, 'package'), archive = pkg + '.zip'; writePackage(pkg);
  const inventory = readInventory(pkg); verifyManifest(pkg, inventory); checks++;
  const entries = entriesFor(pkg, inventory);
  const writeZIP = (items, comment) => { const b = zipFixture(items, comment); fs.writeFileSync(archive, b); fs.writeFileSync(archive + '.sha256', digest(b) + '  ' + path.basename(archive) + '\n'); };
  writeZIP(entries);
  const verified = verifyPackageResources(pkg); assert.equal(verified.numericVersion, '0.1.0.0'); assert.equal(verified.runtimeVersionVerified, false); checks++;
  assert.equal(auditBuiltPackage(pkg).zipMembers, inventory.files.size); checks++;
  writeZIP(entries.map(e => ({ ...e, method: 8, flags: 8 }))); verifyZIPAgainstInventory(archive, 'package', inventory); checks++;

  const extra = (id, value) => { const b = Buffer.alloc(4 + value.length); b.writeUInt16LE(id); b.writeUInt16LE(value.length,2); value.copy(b,4); return b; };
  const unicode = (name, replacement = name, crc = crc32(Buffer.from(name)), version = 1) => { const b = Buffer.alloc(5 + Buffer.byteLength(replacement)); b[0] = version; b.writeUInt32LE(crc,1); b.write(replacement,5); return extra(0x7075,b); };
  const ntfsValue = Buffer.alloc(32); ntfsValue.writeUInt16LE(1,4); ntfsValue.writeUInt16LE(24,6);
  const timestamps = extra(0x5455,Buffer.from([1,0,0,0,0])), ntfs = extra(0x000a,ntfsValue);
  for (const field of ['centralExtra','localExtra']) {
    for (const value of [timestamps,ntfs,unicode(entries[0].name),Buffer.concat([timestamps,ntfs,unicode(entries[0].name)])]) {
      writeZIP(entries.map((e,i)=>i?e:{...e,[field]:value})); verifyZIPAgainstInventory(archive,'package',inventory); checks++;
    }
    const bad = [unicode(entries[0].name,'package/../escape.txt'),unicode(entries[0].name,entries[0].name,0),unicode(entries[0].name,entries[0].name,crc32(Buffer.from(entries[0].name)),2),
      extra(0x0001,Buffer.alloc(16)),extra(0x000d,Buffer.alloc(12)),extra(0xbeef,Buffer.alloc(0)),Buffer.from([1]),Buffer.from([0x75,0x70,5,0,1]),
      Buffer.concat([timestamps,timestamps]),extra(0x5455,Buffer.from([8])),extra(0x5455,Buffer.from([1])),extra(0x000a,Buffer.alloc(32))];
    for (const value of bad) { writeZIP(entries.map((e,i)=>i?e:{...e,[field]:value})); rejects(()=>verifyZIPAgainstInventory(archive,'package',inventory),'unsafe extra field accepted'); }
  }
  const centralTimes = extra(0x5455,Buffer.from([7,0,0,0,0])), localTimes = extra(0x5455,Buffer.alloc(13)); localTimes[4]=7;
  writeZIP(entries.map((e,i)=>i?e:{...e,centralExtra:centralTimes,localExtra:localTimes})); verifyZIPAgainstInventory(archive,'package',inventory); checks++;
  const unicodeEntry = {...entries[0],name:'package/\u6d4b\u8bd5.txt'}, unicodeInventory = {files:new Map(inventory.files),dirs:inventory.dirs};
  const oldName=entries[0].name.slice('package/'.length), originalInfo=unicodeInventory.files.get(oldName);unicodeInventory.files.delete(oldName);unicodeInventory.files.set('\u6d4b\u8bd5.txt',originalInfo);
  writeZIP([unicodeEntry,...entries.slice(1)]); rejects(()=>verifyZIPAgainstInventory(archive,'package',unicodeInventory),'encoding-ambiguous ZIP filename accepted');
  writeZIP([{...unicodeEntry,flags:0x800},...entries.slice(1)]); verifyZIPAgainstInventory(archive,'package',unicodeInventory);checks++;

  const badEntries = [
    entries.map((e, i) => i ? e : { ...e, data: Buffer.from('different payload; fresh ZIP CRC and sidecar') }),
    entries.slice(1), [...entries, { name: 'package/extra.txt', data: 'extra' }], [...entries, entries[0]],
    [...entries, { name: 'package/../escape.txt', data: 'escape' }], [...entries, { name: '/absolute.txt', data: 'absolute' }],
    entries.map((e, i) => i ? e : { ...e, flags: 1 }), entries.map((e, i) => i ? e : { ...e, localName: 'package/different.exe' }),
    entries.map((e, i) => i ? e : { ...e, badCRC: true }), entries.map((e, i) => i ? e : { ...e, attributes: 0xa1ff0000 }),
    entries.map((e, i) => i ? e : { ...e, size: 0xffffffff }), entries.map((e, i) => i ? e : { ...e, size: 300 << 20 })
  ];
  for (const items of badEntries) { writeZIP(items); rejects(() => verifyZIPAgainstInventory(archive, 'package', inventory), 'unsafe/changed ZIP accepted'); }
  writeZIP(entries, 'unreviewed archive comment'); rejects(() => verifyZIPAgainstInventory(archive, 'package', inventory), 'commented ZIP accepted');
  writeZIP(entries); fs.writeFileSync(archive + '.sha256', '0'.repeat(64) + '  ' + path.basename(archive)); rejects(() => verifyArchiveSidecar(archive, digest(fs.readFileSync(archive))), 'wrong sidecar accepted');
  writeZIP(entries);

  const gui = path.join(pkg, 'godusevpn.exe'), originalGUI = fs.readFileSync(gui);
  for (const options of [{ fixedFile: '9.9.9.0' }, { fixedProduct: '9.9.9.0' }, { productString: '0.1.0-old' }, { missingVersion: true }]) {
    fs.writeFileSync(gui, peFixture('godusevpn.exe', '0.01-beta', options)); rejects(() => verifyPackageResources(pkg), 'mismatching RT_VERSION accepted');
  }
  fs.writeFileSync(gui, originalGUI);
  const descriptorPath = path.join(pkg, 'ULTRA_PACKAGE.json'), originalDescriptor = fs.readFileSync(descriptorPath);
  const descriptor = JSON.parse(originalDescriptor); descriptor.version = '9.9.9-beta'; fs.writeFileSync(descriptorPath, JSON.stringify(descriptor)); rejects(() => verifyPackageResources(pkg), 'descriptor-only version echo accepted');
  fs.writeFileSync(descriptorPath, originalDescriptor);
  assert.equal(verifyPackageResources(pkg).version, '0.01-beta'); checks++;
  const consumerDir = path.join(pkg, 'location-consumer'); fs.mkdirSync(consumerDir);
  const consumerExe = path.join(consumerDir, 'GodUseVPN.Ultra.LocationConsumer.exe'), consumerConfig = consumerExe + '.config';
  const consumerManifest = '<assembly><assemblyIdentity name="GodUseVPN.Ultra.LocationConsumer" version="1.0.0.0"/><trustInfo><requestedExecutionLevel level="asInvoker" uiAccess="false"/></trustInfo></assembly>';
  const consumerBytes = peFixture('location-consumer.exe', '0.01-beta', {manifest:consumerManifest});
  const consumerRuntime = '<?xml version="1.0"?><configuration><startup><supportedRuntime version="v4.0" sku=".NETFramework,Version=v4.6.2"/></startup></configuration>';
  fs.writeFileSync(consumerExe, consumerBytes); fs.writeFileSync(consumerConfig, consumerRuntime);
  rejects(() => verifyPackageResources(pkg), 'undeclared independent location consumer accepted');
  const consumerDescriptor = {...JSON.parse(originalDescriptor), locationConsumer:{schemaVersion:1,implementation:'winrt-geolocator',architecture:'amd64',minimumFramework:'4.6.2',nativeRuntimeBundled:false,vmValidated:false}};
  fs.writeFileSync(descriptorPath, JSON.stringify(consumerDescriptor));
  const consumerResult = verifyPackageResources(pkg).locationConsumer;
  assert.equal(consumerResult.executionLevel, 'asInvoker'); assert.equal(consumerResult.runtimeVerified, false); assert.equal(consumerResult.files.length, 2); checks++;
  for (const override of [{minimumFramework:'4.8'},{architecture:'arm64'},{vmValidated:true},{nativeRuntimeBundled:true},{implementation:'fake'}]) {
    fs.writeFileSync(descriptorPath, JSON.stringify({...consumerDescriptor,locationConsumer:{...consumerDescriptor.locationConsumer,...override}}));
    rejects(() => verifyPackageResources(pkg), 'incompatible or false location consumer declaration accepted');
  }
  fs.writeFileSync(descriptorPath, JSON.stringify(consumerDescriptor));
  for (const manifest of [consumerManifest.replace('asInvoker','requireAdministrator'),consumerManifest.replace('LocationConsumer','Other'),consumerManifest.replace('1.0.0.0','9.0.0.0')]) {
    fs.writeFileSync(consumerExe, peFixture('location-consumer.exe', '0.01-beta', {manifest}));
    rejects(() => verifyPackageResources(pkg), 'unexpected location consumer privileges or identity accepted');
  }
  fs.writeFileSync(consumerExe, consumerBytes);
  for (const config of [consumerRuntime.replace('4.6.2','4.8'),consumerRuntime.replace('</configuration>','<runtime><loadFromRemoteSources enabled="true"/></runtime></configuration>')]) {
    fs.writeFileSync(consumerConfig, config); rejects(() => verifyPackageResources(pkg), 'unexpected location runtime configuration accepted');
  }
  fs.writeFileSync(consumerConfig, consumerRuntime);
  fs.writeFileSync(path.join(consumerDir,'unexpected.dll'),'inert'); rejects(() => verifyPackageResources(pkg), 'unexpected consumer dependency accepted'); fs.unlinkSync(path.join(consumerDir,'unexpected.dll'));
  fs.unlinkSync(consumerConfig); rejects(() => verifyPackageResources(pkg), 'missing consumer configuration accepted'); fs.writeFileSync(consumerConfig, consumerRuntime);
  assert.equal(verifyPackageResources(pkg).locationConsumer.runtimeVerified, false); checks++;
  console.log(JSON.stringify({ status: 'passed', checks, scope: 'synthetic PE resources and ZIP members; no product executed' }));
} finally {
  const resolved = path.resolve(temp);
  assert.ok(resolved.startsWith(tempBase + path.sep) && path.basename(resolved).startsWith('ultra-publication-test-'));
  fs.rmSync(resolved, { recursive: true, force: true });
}
