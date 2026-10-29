#!/usr/bin/env node
'use strict';

// QA transfer aid only. It never runs a product, az, an installer or a service.
// Example (explicit local directories; output must not exist):
// node Build-VpsDelta.cjs --base-dir <v1-dir> --new-dir <new-dir> --out-dir <delta-dir>
// node Build-VpsDelta.cjs --self-test
// The SHA256 printed for manifest.json must be supplied independently to the
// PowerShell decoder. A self-consistent manifest is not publisher authentication.
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');
const zlib = require('node:zlib');
const cp = require('node:child_process');

const NAMES = Object.freeze(['godusevpn.exe', 'godusevpn-cli.exe', 'godusevpn-svc.exe']);
const SCHEMA = 'godusevpn-ultra-exe-delta-v1';
const MAGIC = Buffer.from('GUVDEL01', 'ascii');
const BLOCK = 256;
const MAX_FILE = 128 * 1024 * 1024;
const MAX_TOTAL = 384 * 1024 * 1024;
const MAX_COMMANDS = 2000000;
const MAX_MANIFEST = 16384;

function fail(code) { const e = new Error(code); e.code = code; throw e; }
function sha(bytes) { return crypto.createHash('sha256').update(bytes).digest('hex'); }
function u32(n) { const b = Buffer.allocUnsafe(4); b.writeUInt32LE(n); return b; }
function fileBytes(bytes) { if (!Buffer.isBuffer(bytes) || !bytes.length || bytes.length > MAX_FILE) fail('file_size_limit'); }

// Fixed base blocks, weak rolling checksum and exact comparison. A weak hash
// never authorizes a copy; matching bytes are checked before and during extension.
function weak(bytes, offset, count) {
  let a = 0, b = 0;
  for (let i = 0; i < count; i++) { a = (a + bytes[offset + i]) & 0xffff; b = (b + (count - i) * bytes[offset + i]) & 0xffff; }
  return ((b << 16) | a) >>> 0;
}
function buildDelta(base, next) {
  fileBytes(base); fileBytes(next);
  const index = new Map();
  for (let off = 0; off + BLOCK <= base.length; off += BLOCK) {
    const key = weak(base, off, BLOCK), candidates = index.get(key);
    if (!candidates) index.set(key, [off]);
    else if (candidates.length < 16) candidates.push(off);
  }
  const parts = [MAGIC, u32(next.length)];
  let cursor = 0, literalStart = 0, commands = 0;
  let checksum = next.length >= BLOCK ? weak(next, 0, BLOCK) : 0;
  function literal(end) {
    if (end > literalStart) { parts.push(Buffer.from([1]), u32(end - literalStart), next.subarray(literalStart, end)); commands++; }
  }
  while (cursor + BLOCK <= next.length) {
    const candidates = index.get(checksum);
    let match = -1;
    if (candidates) for (const off of candidates) {
      if (base.compare(next, cursor, cursor + BLOCK, off, off + BLOCK) === 0) { match = off; break; }
    }
    if (match >= 0) {
      literal(cursor);
      let length = BLOCK;
      while (match + length < base.length && cursor + length < next.length && base[match + length] === next[cursor + length]) length++;
      parts.push(Buffer.from([0]), u32(match), u32(length)); commands++;
      cursor += length; literalStart = cursor;
      if (cursor + BLOCK <= next.length) checksum = weak(next, cursor, BLOCK);
    } else {
      const outgoing = next[cursor], incoming = next[cursor + BLOCK];
      let a = checksum & 0xffff, b = checksum >>> 16;
      a = (a - outgoing + (incoming === undefined ? 0 : incoming)) & 0xffff;
      b = (b - BLOCK * outgoing + a) & 0xffff;
      checksum = ((b << 16) | a) >>> 0;
      cursor++;
    }
    if (commands > MAX_COMMANDS) fail('command_limit');
  }
  literal(next.length);
  parts.push(Buffer.from([255]));
  if (commands > MAX_COMMANDS) fail('command_limit');
  return Buffer.concat(parts);
}

// Independent bounded decoder used by local synthetic roundtrip tests.
function applyDelta(base, patch, expectedLength) {
  fileBytes(base);
  if (!Number.isInteger(expectedLength) || expectedLength < 1 || expectedLength > MAX_FILE || patch.length > expectedLength * 2 + 32) fail('encoded_size_limit');
  if (patch.length < 13 || !patch.subarray(0, 8).equals(MAGIC) || patch.readUInt32LE(8) !== expectedLength) fail('invalid_header');
  const out = Buffer.alloc(expectedLength);
  let position = 12, written = 0, commands = 0;
  function read32() { if (position + 4 > patch.length) fail('truncated_command'); const n = patch.readUInt32LE(position); position += 4; return n; }
  while (position < patch.length) {
    const op = patch[position++];
    if (op === 255) { if (position !== patch.length || written !== out.length) fail('target_length_mismatch'); return out; }
    if (++commands > MAX_COMMANDS) fail('command_limit');
    if (op === 0) {
      const offset = read32(), length = read32();
      if (!length || offset > base.length || length > base.length - offset || length > out.length - written) fail('copy_bounds');
      base.copy(out, written, offset, offset + length); written += length;
    } else if (op === 1) {
      const length = read32();
      if (!length || length > patch.length - position || length > out.length - written) fail('literal_bounds');
      patch.copy(out, written, position, position + length); written += length; position += length;
    } else fail('invalid_opcode');
  }
  fail('missing_end');
}

function existingLocal(file, directory) {
  const full = path.resolve(file), parsed = path.parse(full);
  if (process.platform === 'win32' && (!/^[a-z]:\\/i.test(full) || full.slice(2).includes(':') || full.startsWith('\\\\'))) fail('local_path_required');
  let current = parsed.root;
  for (const part of full.slice(parsed.root.length).split(path.sep).filter(Boolean)) {
    current = path.join(current, part);
    const stat = fs.lstatSync(current);
    if (stat.isSymbolicLink()) fail('reparse_rejected');
    if (current !== full && !stat.isDirectory()) fail('invalid_parent');
  }
  const stat = fs.lstatSync(full);
  if (stat.isDirectory() !== directory || (!directory && !stat.isFile())) fail('wrong_object_type');
  return full;
}
function readSource(dir, name) {
  const file = existingLocal(path.join(dir, name), false), stat = fs.statSync(file);
  if (!stat.size || stat.size > MAX_FILE || stat.nlink !== 1) fail('file_size_or_link_limit');
  const bytes = fs.readFileSync(file); fileBytes(bytes);
  if (bytes.length !== stat.size) fail('source_changed');
  return bytes;
}
function buildPackage(baseDirectory, newDirectory, outputDirectory) {
  const base = existingLocal(baseDirectory, true), next = existingLocal(newDirectory, true);
  const output = path.resolve(outputDirectory);
  existingLocal(path.dirname(output), true);
  if (fs.existsSync(output)) fail('output_exists');
  const files = [], payloads = [];
  let total = 0;
  for (const name of NAMES) {
    const original = readSource(base, name), updated = readSource(next, name);
    total += updated.length; if (total > MAX_TOTAL) fail('total_size_limit');
    const encoded = buildDelta(original, updated);
    // Builder verifies its generated instruction stream before publishing it.
    if (!applyDelta(original, encoded, updated.length).equals(updated)) fail('roundtrip_mismatch');
    const compressed = zlib.gzipSync(encoded, { level: 9 });
    const patchFile = name + '.delta.gz';
    files.push({ name, baseSha256: sha(original), newSha256: sha(updated), baseLength: original.length, newLength: updated.length, patchFile, patchSha256: sha(compressed), patchLength: compressed.length, encodedLength: encoded.length });
    payloads.push([patchFile, compressed]);
  }
  const manifest = Buffer.from(JSON.stringify({ schema: SCHEMA, blockBytes: BLOCK, files }, null, 2) + '\n');
  if (manifest.length > MAX_MANIFEST) fail('manifest_size_limit');
  fs.mkdirSync(output); // exclusive, no recursive merge and no existing ACL changes
  for (const [name, bytes] of payloads) fs.writeFileSync(path.join(output, name), bytes, { flag: 'wx', mode: 0o600 });
  fs.writeFileSync(path.join(output, 'manifest.json'), manifest, { flag: 'wx', mode: 0o600 });
  return { ok: true, files: files.length, baseBytes: files.reduce((n, x) => n + x.baseLength, 0), newBytes: total, patchBytes: payloads.reduce((n, x) => n + x[1].length, 0), manifestSha256: sha(manifest) };
}

function selfTest() {
  let seed = 0x21347856;
  const random = n => { const b = Buffer.alloc(n); for (let i = 0; i < n; i++) { seed ^= seed << 13; seed ^= seed >>> 17; seed ^= seed << 5; b[i] = seed & 255; } return b; };
  const baseBytes = random(32768), newBytes = Buffer.concat([random(37), baseBytes.subarray(0, 14000), random(600), baseBytes.subarray(14000), random(123)]);
  for (const [a, b] of [[baseBytes, newBytes], [baseBytes, baseBytes], [Buffer.from('a'), Buffer.from('changed')], [Buffer.alloc(4096), Buffer.alloc(8192)], [baseBytes, random(65536)]]) {
    if (!applyDelta(a, buildDelta(a, b), b.length).equals(b)) fail('self_test_roundtrip');
  }
  const good = buildDelta(baseBytes, newBytes);
  const malformed = [good.subarray(0, 10), Buffer.concat([MAGIC, u32(1), Buffer.from([0]), u32(baseBytes.length), u32(1), Buffer.from([255])]), Buffer.concat([MAGIC, u32(1), Buffer.from([1]), u32(2), Buffer.from([1, 2, 255])]), Buffer.concat([MAGIC, u32(1), Buffer.from([42, 255])]), Buffer.concat([good, Buffer.from([0])])];
  for (const bytes of malformed) { let rejected = false; try { applyDelta(baseBytes, bytes, bytes === good ? newBytes.length : 1); } catch (_) { rejected = true; } if (!rejected) fail('self_test_malformed'); }
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'ultra-delta-selftest-'));
  const knownFiles = [], knownDirs = [root];
  const mkdir = name => { const dir = path.join(root, name); fs.mkdirSync(dir); knownDirs.push(dir); return dir; };
  const write = (dir, name, bytes) => { const file = path.join(dir, name); fs.writeFileSync(file, bytes, { flag: 'wx' }); knownFiles.push(file); };
  try {
    const base = mkdir('base'), next = mkdir('new'), pack = path.join(root, 'package');
    NAMES.forEach((name, i) => { write(base, name, Buffer.concat([baseBytes, Buffer.from([i])])); write(next, name, Buffer.concat([newBytes, Buffer.from([i])])); });
    buildPackage(base, next, pack); knownDirs.push(pack);
    for (const name of ['manifest.json', ...NAMES.map(n => n + '.delta.gz')]) knownFiles.push(path.join(pack, name));
    const manifest = fs.readFileSync(path.join(pack, 'manifest.json'));
    let psCases = 0;
    if (process.platform === 'win32') {
      const shell = path.join(process.env.SystemRoot || 'C:\\Windows', 'System32', 'WindowsPowerShell', 'v1.0', 'powershell.exe');
      const script = path.join(__dirname, 'Apply-VpsDelta.ps1');
      function run(dir, expected, pass, code, existing) {
        const output = existing || path.join(root, 'delta-' + crypto.randomBytes(16).toString('hex'));
        const args = ['-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', script, '-BaseDirectory', base, '-DeltaDirectory', dir, '-OutputDirectory', output, '-ExpectedManifestSha256', expected];
        const result = cp.spawnSync(shell, args, { shell: false, encoding: 'utf8', timeout: 120000, windowsHide: true });
        let receipt; try { receipt = JSON.parse(result.stdout.trim()); } catch (_) { fail('self_test_ps_receipt'); }
        if ((result.status === 0) !== pass || receipt.ready !== pass || (code && receipt.error !== code)) fail('self_test_ps_' + (receipt.error || code || 'roundtrip'));
        if (pass) {
          knownDirs.push(output);
          for (const name of NAMES) { knownFiles.push(path.join(output, name)); if (!fs.readFileSync(path.join(output, name)).equals(fs.readFileSync(path.join(next, name)))) fail('self_test_ps_bytes'); }
          if (fs.readdirSync(output).length !== 3) fail('self_test_ps_extra_output');
        } else if (!existing && fs.existsSync(output)) fail('self_test_rejected_output_created');
        psCases++;
      }
      run(pack, sha(manifest), true);
      const existing = mkdir('delta-' + crypto.randomBytes(16).toString('hex'));
      write(existing,'sentinel.txt',Buffer.from('keep'));
      run(pack,sha(manifest),false,'output_exists',existing);
      if (fs.readdirSync(existing).length!==1 || fs.readFileSync(path.join(existing,'sentinel.txt'),'utf8')!=='keep') fail('self_test_existing_output_changed');
      function damaged(label, mutate, code) {
        const dir = mkdir(label), data = JSON.parse(manifest);
        const patches = new Map(); for (const name of NAMES) patches.set(name + '.delta.gz', fs.readFileSync(path.join(pack, name + '.delta.gz')));
        mutate(data, patches);
        const json = Buffer.from(JSON.stringify(data)); write(dir, 'manifest.json', json);
        for (const [name, bytes] of patches) write(dir, name, bytes);
        run(dir, sha(json), false, code);
      }
      damaged('duplicate', data => data.files[1] = data.files[0], 'invalid_manifest');
      damaged('oversize', data => data.files[0].newLength = MAX_FILE + 1, 'invalid_manifest');
      damaged('block-type', data => data.blockBytes = '256', 'invalid_manifest');
      damaged('base-hash', data => data.files[0].baseSha256 = '0'.repeat(64), 'base_hash_mismatch');
      damaged('decompression-size', data => data.files[0].encodedLength = 13, 'gzip_footer_mismatch');
      damaged('bad-hash', (data, patches) => { const p = Buffer.from(patches.get(data.files[0].patchFile)); p[0] ^= 1; patches.set(data.files[0].patchFile, p); }, 'patch_hash_mismatch');
      damaged('copy-bounds', (data, patches) => { const f = data.files[0], encoded = Buffer.concat([MAGIC,u32(f.newLength),Buffer.from([0]),u32(f.baseLength),u32(1),Buffer.from([255])]), gzip = zlib.gzipSync(encoded); patches.set(f.patchFile,gzip); f.encodedLength=encoded.length;f.patchLength=gzip.length;f.patchSha256=sha(gzip); }, 'copy_bounds');
      damaged('truncated-literal', (data, patches) => { const f=data.files[0],encoded=Buffer.concat([MAGIC,u32(f.newLength),Buffer.from([1]),u32(f.newLength),Buffer.from([1,255])]),gzip=zlib.gzipSync(encoded);patches.set(f.patchFile,gzip);f.encodedLength=encoded.length;f.patchLength=gzip.length;f.patchSha256=sha(gzip); }, 'literal_bounds');
      damaged('bad-target', data => data.files[0].newSha256='0'.repeat(64), 'target_hash_mismatch');
      for (const kind of ['truncate-one','truncate-footer','append-garbage','bad-crc','bad-isize']) {
        damaged(kind,(data,patches)=>{
          const f=data.files[0];let bytes=Buffer.from(patches.get(f.patchFile));
          if (kind==='truncate-one') bytes=bytes.subarray(0,bytes.length-1);
          else if (kind==='truncate-footer') bytes=bytes.subarray(0,bytes.length-8);
          else if (kind==='append-garbage') bytes=Buffer.concat([bytes,Buffer.from([9,8,7])]);
          else if (kind==='bad-crc') bytes[bytes.length-8]^=1;
          else bytes[bytes.length-1]^=1;
          patches.set(f.patchFile,bytes);f.patchLength=bytes.length;f.patchSha256=sha(bytes);
        },kind==='bad-crc'?null:'gzip_footer_mismatch');
      }
      const absent = path.join(base,NAMES[0]); fs.renameSync(absent,absent+'.held'); knownFiles[knownFiles.indexOf(absent)]=absent+'.held';
      run(pack,sha(manifest),false,'missing_base');
      fs.renameSync(absent+'.held',absent);knownFiles[knownFiles.indexOf(absent+'.held')]=absent;
    }
    return { ok: true, algorithmRoundtrips: 5, malformedStreamsRejected: malformed.length, powerShellCases: psCases, productExecuted: false, networkRequested: false };
  } finally {
    // Delete only explicitly tracked synthetic files, then empty known dirs.
    for (const file of knownFiles.reverse()) { try { fs.unlinkSync(file); } catch (_) {} }
    for (const dir of knownDirs.reverse()) { try { fs.rmdirSync(dir); } catch (_) {} }
  }
}

function main(args) {
  if (args.length === 1 && args[0] === '--self-test') return selfTest();
  const options = {};
  for (let i = 0; i < args.length; i += 2) {
    const key = args[i];
    if (!['--base-dir','--new-dir','--out-dir'].includes(key) || !args[i+1] || options[key]) fail('invalid_arguments');
    options[key] = args[i+1];
  }
  if (Object.keys(options).length !== 3) fail('explicit_directories_required');
  return buildPackage(options['--base-dir'],options['--new-dir'],options['--out-dir']);
}
module.exports = { buildDelta, applyDelta, buildPackage, NAMES, MAX_FILE };
if (require.main === module) {
  try { process.stdout.write(JSON.stringify(main(process.argv.slice(2)))+'\n'); }
  catch (error) { const code = error.code && /^[a-z_]+$/.test(error.code) ? error.code : 'delta_build_failed'; process.stderr.write(JSON.stringify({ok:false,error:code})+'\n');process.exitCode=1; }
}
