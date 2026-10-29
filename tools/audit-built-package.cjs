'use strict';
const fs = require('node:fs'), path = require('node:path'), crypto = require('node:crypto'), zlib = require('node:zlib'), assert = require('node:assert/strict');
const { verifyPackageResources } = require('./verify-ultra-package.cjs');
const root = path.resolve(__dirname, '..');
const limits = { archive: 512 << 20, file: 256 << 20, total: 512 << 20, entries: 20000 };
const digest = b => crypto.createHash('sha256').update(b).digest('hex');

function safePayloadPath(name) {
  if (!name || name !== name.trim() || name.includes('\\') || name.includes(':') || name.includes('\0') || name.startsWith('/')) return false;
  return name.split('/').every(part => part && part !== '.' && part !== '..' && !/[. ]$/.test(part) && !/[\x00-\x1f*?"<>|]/.test(part) && !/^(?:CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|COM[1-9¹²³]|LPT[1-9¹²³])(?:\.|$)/i.test(part));
}

function readInventory(pkg) {
  const stat = fs.lstatSync(pkg);
  if (!stat.isDirectory() || stat.isSymbolicLink()) throw Error('Package root must be a regular directory');
  const files = new Map(), dirs = new Set(['']), names = new Set();
  let total = 0, count = 0;
  function walk(dir, depth) {
    if (depth > 64) throw Error('Package depth limit');
    for (const name of fs.readdirSync(dir)) {
      const p = path.join(dir, name), s = fs.lstatSync(p), relative = path.relative(pkg, p).split(path.sep).join('/');
      if (++count > limits.entries || !safePayloadPath(relative) || names.has(relative.toLowerCase())) throw Error('Unsafe, duplicate or excessive payload paths');
      names.add(relative.toLowerCase());
      if (s.isSymbolicLink()) throw Error('Reparse/symlink in package');
      if (s.isDirectory()) { dirs.add(relative); walk(p, depth + 1); }
      else if (s.isFile()) {
        if (s.size > limits.file || (total += s.size) > limits.total) throw Error('Package file/total size limit');
        files.set(relative, { bytes: s.size, sha256: digest(fs.readFileSync(p)) });
      } else throw Error('Non-regular payload');
    }
  }
  walk(pkg, 0);
  return { files, dirs };
}

function verifyManifest(pkg, inventory) {
  if ((inventory.files.get('SHA256SUMS')?.bytes ?? Infinity) > 4 << 20) throw Error('Manifest size limit');
  const manifest = new Map(), folded = new Set();
  for (const line of fs.readFileSync(path.join(pkg, 'SHA256SUMS'), 'utf8').trimEnd().split(/\r?\n/)) {
    const match = /^([a-f0-9]{64})  (.+)$/.exec(line);
    assert.ok(match, 'Malformed manifest');
    const name = match[2];
    assert.ok(safePayloadPath(name) && name.toLowerCase() !== 'sha256sums' && !folded.has(name.toLowerCase()), 'Unsafe/duplicate manifest path');
    assert.equal(inventory.files.get(name)?.sha256, match[1], 'Payload mismatch: ' + name);
    manifest.set(name, match[1]); folded.add(name.toLowerCase());
  }
  assert.deepEqual([...inventory.files.keys()].filter(p => p !== 'SHA256SUMS').sort(), [...manifest.keys()].sort(), 'Extra or missing payload');
  for (const required of ['godusevpn.exe', 'godusevpn-cli.exe', 'godusevpn-svc.exe', 'ULTRA_PACKAGE.json', 'Install-Ultra.ps1', 'Uninstall-Ultra.ps1', 'browser-extension/manifest.json']) assert.ok(manifest.has(required), 'Missing required payload: ' + required);
  return manifest;
}

const crcTable = Uint32Array.from({ length: 256 }, (_, i) => {
  let c = i; for (let n = 0; n < 8; n++) c = c & 1 ? 0xedb88320 ^ c >>> 1 : c >>> 1; return c >>> 0;
});
function crc32(b) { let c = 0xffffffff; for (const n of b) c = crcTable[(c ^ n) & 255] ^ c >>> 8; return (c ^ 0xffffffff) >>> 0; }

// PKWARE APPNOTE sections 4.5.5 and 4.6.9:
// https://pkware.cachefly.net/webdocs/casestudies/APPNOTE.TXT
// Accept metadata-only timestamps and an exactly equivalent Unicode path.
// Unknown fields can alter extraction semantics (links/ACLs/paths/encryption),
// so skipping their bytes would not prove the reviewed extraction tree.
function verifyExtraFields(extra, nameBytes, central) {
  const seen = new Set();
  for (let off = 0; off < extra.length;) {
    if (off + 4 > extra.length) throw Error('Truncated ZIP extra field');
    const id = extra.readUInt16LE(off), size = extra.readUInt16LE(off + 2);
    off += 4;
    if (off + size > extra.length || seen.has(id)) throw Error('Truncated/duplicate ZIP extra field');
    seen.add(id);
    const value = extra.subarray(off, off + size); off += size;
    if (id === 0x7075) {
      if (size < 5 || value[0] !== 1 || value.readUInt32LE(1) !== crc32(nameBytes) || !value.subarray(5).equals(nameBytes)) throw Error('ZIP Unicode path differs from reviewed name');
    } else if (id === 0x000a) {
      // Reserved DWORD + the sole documented NTFS tag, three FILETIMEs.
      if (size !== 32 || value.readUInt32LE(0) !== 0 || value.readUInt16LE(4) !== 1 || value.readUInt16LE(6) !== 24) throw Error('Unsupported NTFS ZIP attributes');
    } else if (id === 0x5455) {
      if (!size || value[0] & ~7) throw Error('Invalid ZIP timestamp flags');
      const flags = value[0], times = (flags & 1 ? 1 : 0) + (flags & 2 ? 1 : 0) + (flags & 4 ? 1 : 0);
      // Central headers may omit access/creation times while retaining flags.
      if (size !== 1 + 4 * times && !(central && flags & 1 && size === 5)) throw Error('Invalid ZIP timestamp length');
    } else {
      throw Error('Unsupported ZIP extra field: ' + id.toString(16));
    }
  }
}

// Compare the actual uploaded archive, not just an adjacent directory and a
// self-consistent ZIP checksum. No extraction, shell or product process runs.
function verifyZIPAgainstInventory(archive, packageName, inventory) {
  const stat = fs.lstatSync(archive);
  if (!stat.isFile() || stat.isSymbolicLink() || stat.size > limits.archive) throw Error('Archive type/size limit');
  const b = fs.readFileSync(archive);
  let eocd = -1;
  for (let off = b.length - 22; off >= Math.max(0, b.length - 65557); off--) {
    if (b.readUInt32LE(off) === 0x06054b50 && off + 22 + b.readUInt16LE(off + 20) === b.length) { eocd = off; break; }
  }
  if (eocd < 0) throw Error('ZIP end record missing');
  const count = b.readUInt16LE(eocd + 10), centralSize = b.readUInt32LE(eocd + 12), centralStart = b.readUInt32LE(eocd + 16);
  if (b.readUInt16LE(eocd + 4) || b.readUInt16LE(eocd + 6) || b.readUInt16LE(eocd + 8) !== count || b.readUInt16LE(eocd + 20) || count > limits.entries || count === 65535 || centralStart === 0xffffffff || centralSize === 0xffffffff || centralStart + centralSize !== eocd) throw Error('ZIP64, split, commented or malformed ZIP is unsupported');
  const seen = new Set(), folded = new Set(), ranges = [];
  let off = centralStart, total = 0, members = 0;
  const decodeName = bytes => {
    const name = bytes.toString('utf8');
    if (!Buffer.from(name, 'utf8').equals(bytes)) throw Error('Invalid ZIP UTF-8 name');
    return name;
  };
  for (let i = 0; i < count; i++) {
    if (off + 46 > eocd || b.readUInt32LE(off) !== 0x02014b50) throw Error('Invalid ZIP central directory');
    const flags = b.readUInt16LE(off + 8), method = b.readUInt16LE(off + 10), crc = b.readUInt32LE(off + 16), compressed = b.readUInt32LE(off + 20), uncompressed = b.readUInt32LE(off + 24);
    const nameLength = b.readUInt16LE(off + 28), extraLength = b.readUInt16LE(off + 30), commentLength = b.readUInt16LE(off + 32), disk = b.readUInt16LE(off + 34), attributes = b.readUInt32LE(off + 38), local = b.readUInt32LE(off + 42);
    if (off + 46 + nameLength + extraLength + commentLength > eocd || !nameLength || commentLength || disk || flags & 1 || flags & 0x2040 || ![0, 8].includes(method) || [compressed, uncompressed, local].includes(0xffffffff)) throw Error('Unsupported or unbounded ZIP entry');
    const bytes = b.subarray(off + 46, off + 46 + nameLength), name = decodeName(bytes), directory = name.endsWith('/');
    if (!(flags & 0x800) && bytes.some(byte => byte >= 0x80)) throw Error('Non-ASCII ZIP name requires the UTF-8 flag');
    verifyExtraFields(b.subarray(off + 46 + nameLength, off + 46 + nameLength + extraLength), bytes, true);
    const mode = attributes >>> 16 & 0xf000;
    if (mode === 0xa000 || attributes & 0x400 || mode && ![0x4000, 0x8000].includes(mode) || (mode === 0x4000 || attributes & 0x10) && !directory) throw Error('Non-regular/reparse ZIP member');
    if (!name.startsWith(packageName + '/')) throw Error('ZIP member outside the reviewed package root');
    const relative = name.slice(packageName.length + 1).replace(/\/$/, '');
    if (relative && !safePayloadPath(relative) || !relative && !directory || folded.has(name.toLowerCase())) throw Error('Unsafe or duplicate ZIP member');
    folded.add(name.toLowerCase());
    if (directory ? !inventory.dirs.has(relative) : !inventory.files.has(relative)) throw Error('Extra ZIP member: ' + relative);
    if (uncompressed > limits.file || (total += uncompressed) > limits.total) throw Error('ZIP expansion limit');
    if (local + 30 > centralStart || b.readUInt32LE(local) !== 0x04034b50) throw Error('Invalid ZIP local record');
    const localNameLength = b.readUInt16LE(local + 26), localExtra = b.readUInt16LE(local + 28), dataStart = local + 30 + localNameLength + localExtra, dataEnd = dataStart + compressed;
    if (dataStart > centralStart || dataEnd > centralStart || b.readUInt16LE(local + 6) !== flags || b.readUInt16LE(local + 8) !== method || !b.subarray(local + 30, local + 30 + localNameLength).equals(bytes)) throw Error('ZIP local/central mismatch');
    verifyExtraFields(b.subarray(local + 30 + localNameLength, dataStart), bytes, false);
    let recordEnd = dataEnd;
    if (flags & 8) {
      let descriptor = dataEnd;
      if (descriptor + 4 <= centralStart && b.readUInt32LE(descriptor) === 0x08074b50) descriptor += 4;
      if (descriptor + 12 > centralStart || b.readUInt32LE(descriptor) !== crc || b.readUInt32LE(descriptor + 4) !== compressed || b.readUInt32LE(descriptor + 8) !== uncompressed) throw Error('ZIP data descriptor mismatch');
      recordEnd = descriptor + 12;
    } else if (b.readUInt32LE(local + 14) !== crc || b.readUInt32LE(local + 18) !== compressed || b.readUInt32LE(local + 22) !== uncompressed) throw Error('ZIP local size/CRC mismatch');
    ranges.push([local, recordEnd]);
    const stored = b.subarray(dataStart, dataEnd);
    const data = method === 0 ? stored : zlib.inflateRawSync(stored, { maxOutputLength: uncompressed + 1 });
    if (data.length !== uncompressed || crc32(data) !== crc) throw Error('ZIP content length/CRC mismatch');
    if (directory) { if (data.length) throw Error('ZIP directory has data'); }
    else {
      const expected = inventory.files.get(relative);
      if (data.length !== expected.bytes || digest(data) !== expected.sha256) throw Error('ZIP bytes differ from reviewed payload: ' + relative);
      seen.add(relative); members++;
    }
    off += 46 + nameLength + extraLength + commentLength;
  }
  if (off !== centralStart + centralSize) throw Error('ZIP central-directory size mismatch');
  ranges.sort((a, b) => a[0] - b[0]);
  for (let i = 1; i < ranges.length; i++) if (ranges[i][0] < ranges[i - 1][1]) throw Error('Overlapping ZIP records');
  assert.deepEqual([...seen].sort(), [...inventory.files.keys()].sort(), 'Missing ZIP payload');
  return { members, zipSHA256: digest(b), zipBytes: b.length };
}

function verifyArchiveSidecar(archive, sha256) {
  const sidecar = fs.readFileSync(archive + '.sha256', 'utf8').trimEnd();
  const match = /^([a-f0-9]{64})  ([^\r\n]+)$/.exec(sidecar);
  assert.ok(match && match[2] === path.basename(archive), 'Invalid archive checksum sidecar');
  assert.equal(match[1], sha256, 'Archive checksum mismatch');
}

function auditBuiltPackage(pkg) {
  pkg = path.resolve(pkg);
  const inventory = readInventory(pkg), manifest = verifyManifest(pkg, inventory), pe = verifyPackageResources(pkg);
  const gui = fs.readFileSync(path.join(pkg, 'godusevpn.exe'));
  const assets = ['app.js', 'i18n.js', 'index.html', 'style.css', 'privacy.js', 'privacy.css'].map(name => {
    const b = fs.readFileSync(path.join(root, 'web/dist', name));
    assert.ok(gui.includes(b), 'Stale embedded frontend: ' + name); return { name, sha256: digest(b) };
  });
  const archive = pkg + '.zip', zip = verifyZIPAgainstInventory(archive, path.basename(pkg), inventory);
  verifyArchiveSidecar(archive, zip.zipSHA256);
  return { version: pe.version, scope: 'package directory, actual ZIP members/bytes, PE resources and frontend; no runtime/installed behavior or traffic claim', payloadFiles: manifest.size, zipMembers: zip.members, zipSHA256: zip.zipSHA256, binaries: pe.binaries, runtimeVersionVerified: false, assets };
}

module.exports = { limits, digest, safePayloadPath, readInventory, verifyManifest, crc32, verifyZIPAgainstInventory, verifyArchiveSidecar, auditBuiltPackage };
if (require.main === module) {
  if (!process.argv[2]) throw Error('Pass the built package directory.');
  const pkg = path.resolve(process.argv[2]), result = auditBuiltPackage(pkg);
  fs.writeFileSync(pkg + '.zip.audit.json', JSON.stringify(result, null, 2) + '\n');
  console.log(JSON.stringify(result, null, 2));
}
