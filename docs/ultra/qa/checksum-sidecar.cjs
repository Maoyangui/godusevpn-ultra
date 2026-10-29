'use strict';
// Select only the real sidecar writer from the build AST. Never run the build.
// Usage: node checksum-sidecar.cjs <powershell.exe> <GNU-sha256sum-executable>
const fs = require('node:fs'), os = require('node:os'), path = require('node:path');
const cp = require('node:child_process'), crypto = require('node:crypto'), assert = require('node:assert/strict');
const root = path.resolve(__dirname, '../../..');
const powershell = process.argv[2], checksum = process.argv[3];
if (!powershell || !checksum) throw Error('Pass PowerShell and GNU sha256sum executable paths.');
const fixture = fs.mkdtempSync(path.join(os.tmpdir(), 'ultra-checksum-'));
try {
  const archive = path.join(fixture, 'inert-fixture.zip');
  const bytes = Buffer.from('Inert checksum fixture; this is not a product ZIP.\n', 'ascii');
  fs.writeFileSync(archive, bytes, {flag: 'wx'});
  const script = String.raw`
$ErrorActionPreference = 'Stop'
# Isolate built-in modules from a parent PowerShell 7 process's module path.
$env:PSModulePath = [IO.Path]::Combine($PSHOME, 'Modules')
$tokens = $null; $parseErrors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($env:ULTRA_CHECKSUM_BUILD, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count) { throw 'Build source has parse errors.' }
$writers = @($ast.FindAll({param($n)
    $n -is [Management.Automation.Language.InvokeMemberExpressionAst] -and
    $n.Static -and $n.Expression.Extent.Text -eq '[IO.File]' -and
    $n.Member.Value -eq 'WriteAllText'
}, $true))
if ($writers.Count -ne 1 -or $writers[0].Arguments.Count -ne 3 -or
    $writers[0].Arguments[0].Extent.Text -cne '"$archive.sha256"') { throw 'Expected one fixed sidecar writer.' }
$archive = $env:ULTRA_CHECKSUM_FIXTURE
& ([scriptblock]::Create($writers[0].Extent.Text))
`;
  const run = cp.spawnSync(powershell, ['-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(script, 'utf16le').toString('base64')], {
    env: {...process.env, ULTRA_CHECKSUM_BUILD: path.join(root, 'build-ultra.ps1'), ULTRA_CHECKSUM_FIXTURE: archive},
    encoding: 'utf8', windowsHide: true, timeout: 15000
  });
  assert.equal(run.status, 0, 'AST-selected sidecar writer must succeed.');
  const hash = crypto.createHash('sha256').update(bytes).digest('hex');
  assert.deepEqual(fs.readFileSync(archive + '.sha256'), Buffer.from(hash + '  inert-fixture.zip\n', 'ascii'));
  const checked = cp.spawnSync(checksum, ['--check', 'inert-fixture.zip.sha256'], {cwd: fixture, encoding: 'utf8', windowsHide: true, timeout: 15000});
  assert.equal(checked.status, 0, 'GNU sha256sum must accept the generated sidecar.');
  assert.equal(checked.stderr, '');
  assert.match(checked.stdout, /inert-fixture\.zip: OK/);
  console.log(JSON.stringify({passed: true, bom: false, cr: false, finalLF: true, gnuVerified: true, scope: 'AST-selected writer and inert Temp file only; no build/product/ZIP changes'}));
} finally {
  // Delete only this exact new fixture; never any project or product directory.
  if (path.dirname(fixture) !== path.resolve(os.tmpdir()) || !/^ultra-checksum-[A-Za-z0-9]+$/.test(path.basename(fixture))) throw Error('Unsafe fixture cleanup.');
  fs.rmSync(fixture, {recursive: true});
}
