#!/usr/bin/env python3
"""Stage the fixed QA2 delta-transfer ZIP through Azure Cloud Shell; never install or run a product. This is a transfer aid, not the release package.

Requires Python 3.10+. Run manually in an authenticated Bash Cloud Shell with an explicit target:
  python3 Stage-VpsPackage.py --zip ./preview.zip \
    --subscription <subscription-guid> --resource-group <resource-group> \
    --vm-name <vm-resource-name> --stage-id <fresh-32-hex-uuid4>

Use exactly the same stage-id to resume verified parts. --dry-run only checks
the local ZIP and generated request sizes. --self-test uses in-memory fakes and
temporary local files; it never runs az or touches Windows settings.

Remote writes are confined to the fixed KnownFolder ProgramData child
GodUseVPN-Ultra-QA/<stage-id>. They comprise stage.json, immutable part files,
a uniquely named merge file and preview.zip. Existing bytes are never replaced.
Incomplete/conflicting files remain for review: use a fresh stage-id rather than
deleting unknown files. No extraction, product execution, install or network
configuration is performed. Azure itself may retain Run Command script/log data;
only stage this approved test-build ZIP, never credentials or private user files.
The local ProgramData KnownFolder and its OS ancestors must already be trusted;
reparse rejection and QA ACL checks do not audit the entire machine's security.

Prerequisites/limits checked against Microsoft's documentation:
https://learn.microsoft.com/en-us/cli/azure/vm/run-command#az-vm-run-command-invoke
https://learn.microsoft.com/en-us/azure/virtual-machines/windows/run-command
https://learn.microsoft.com/en-us/azure/azure-resource-manager/management/azure-subscription-service-limits
https://learn.microsoft.com/en-us/dotnet/api/system.io.directory.createdirectory?view=netframework-4.8.1

Run Command needs a ready VM agent and Azure connectivity, runs as SYSTEM on
Windows, allows one operation at a time and exposes only the final 4096 output
bytes. A local timeout does NOT cancel the remote command. Wait for it to finish
before resuming. This tool never automatically retries an unknown outcome.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import uuid


EXPECTED_ZIP_SHA256 = "af5513ea94de4118e2f72bc650da4fee2d18610925a38a3b17dc421b6f885b7f"
CHUNK_BYTES = 2 * 1024 * 1024
MAX_PARTS = 256
# Below ARM's documented 4,194,304-byte request limit, with spare JSON overhead.
MAX_REQUEST_BYTES = 4_000_000
CALL_TIMEOUT_SECONDS = 900
RECEIPT_PREFIX = "ULTRA_STAGE_V1:"
REMOTE_ERROR_CODES = frozenset({
    "reparse_rejected", "wrong_object_type", "untrusted_owner", "directory_acl_not_private",
    "untrusted_write_acl", "existing_file_size_conflict", "existing_file_hash_conflict",
    "system_run_command_required", "invalid_stage", "invalid_programdata", "invalid_manifest",
    "invalid_manifest_part", "invalid_manifest_size", "invalid_part_index",
    "payload_size_mismatch", "payload_hash_mismatch", "remote_io_failure",
})


class StageError(Exception):
    """Only fixed, non-sensitive status codes may reach the terminal."""


class QuietParser(argparse.ArgumentParser):
    def error(self, message):  # Do not echo arbitrary argv values on errors.
        raise StageError("invalid_arguments_use_help")


def validate_target(subscription: str, resource_group: str, vm_name: str,
                    stage_id: str) -> dict[str, str]:
    try:
        parsed = uuid.UUID(subscription)
    except (ValueError, AttributeError):
        raise StageError("invalid_subscription_guid") from None
    if str(parsed) != subscription.lower() or parsed.int == 0:
        raise StageError("invalid_subscription_guid")
    # Conservative resource-name subset. No leading option, whitespace, slash,
    # command substitution, quote or control characters can enter the az argv.
    if not re.fullmatch(r"[A-Za-z0-9_][A-Za-z0-9_.()-]{0,89}", resource_group) or resource_group.endswith("."):
        raise StageError("invalid_resource_group_name")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,63}", vm_name) or vm_name.endswith((".", "-")):
        raise StageError("invalid_vm_resource_name")
    if not re.fullmatch(r"[a-f0-9]{32}", stage_id):
        raise StageError("stage_id_requires_lowercase_uuid4_hex")
    stage = uuid.UUID(hex=stage_id)
    if stage.version != 4 or stage.variant != uuid.RFC_4122:
        raise StageError("stage_id_requires_lowercase_uuid4_hex")
    return {"subscription": str(parsed), "resource_group": resource_group,
            "vm_name": vm_name, "stage_id": stage_id}


def inspect_stream(stream, expected_hash: str) -> dict:
    total = hashlib.sha256()
    parts = []
    size = 0
    while True:
        block = stream.read(CHUNK_BYTES)
        if not block:
            break
        if len(parts) >= MAX_PARTS:
            raise StageError("local_package_too_large")
        size += len(block)
        total.update(block)
        parts.append({"size": len(block), "sha256": hashlib.sha256(block).hexdigest()})
    if not size or total.hexdigest() != expected_hash:
        raise StageError("local_zip_sha256_mismatch_no_remote_writes")
    return {"version": 1, "kind": "GodUseVPN-Ultra-QA-stage", "sha256": expected_hash,
            "size": size, "chunkBytes": CHUNK_BYTES, "parts": parts}


def compact_json(value) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True)


# Windows PowerShell 5/.NET Framework's DirectorySecurity overload creates the
# DACL atomically. If it is unavailable, stop; do not fall back to mkdir+chmod.
# All substitutions below are validated hex, decimal or base64 data, never paths
# or user-provided commands. VM identity stays in subprocess argv, not this text.
REMOTE_COMMON = r"""
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$VerbosePreference = 'SilentlyContinue'
Set-StrictMode -Version 2
$Nonce = '__NONCE__'
$Operation = '__OP__'
$Index = __INDEX__
$Count = __COUNT__
$StageID = '__STAGE__'
$FixedZipHash = 'af5513ea94de4118e2f72bc650da4fee2d18610925a38a3b17dc421b6f885b7f'
function Reply([bool]$OK, [string]$Status) {
    $value = @{v=1; ok=$OK; op=$Operation; index=$Index; count=$Count; status=$Status; sha256Match=$OK}
    [Console]::Out.WriteLine('ULTRA_STAGE_V1:' + $Nonce + ':' + ($value | ConvertTo-Json -Compress))
}
function Assert-NoReparse([string]$Path) {
    $current = [IO.Path]::GetFullPath($Path)
    while ($current) {
        $entry = Get-Item -LiteralPath $current -Force -ErrorAction Stop
        if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'reparse_rejected' }
        $parent = [IO.Path]::GetDirectoryName($current.TrimEnd('\'))
        if (-not $parent -or $parent -eq $current) { break }
        $current = $parent
    }
}
function Assert-Private([string]$Path, [bool]$Directory) {
    Assert-NoReparse $Path
    $entry = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    if ([bool]$entry.PSIsContainer -ne $Directory) { throw 'wrong_object_type' }
    $acl = Get-Acl -LiteralPath $Path -ErrorAction Stop
    $owner = $acl.GetOwner([Security.Principal.SecurityIdentifier]).Value
    if ($owner -notin @('S-1-5-18','S-1-5-32-544')) { throw 'untrusted_owner' }
    if ($Directory -and -not $acl.AreAccessRulesProtected) { throw 'directory_acl_not_private' }
    $write = [Security.AccessControl.FileSystemRights]::WriteData -bor
             [Security.AccessControl.FileSystemRights]::AppendData -bor
             [Security.AccessControl.FileSystemRights]::WriteExtendedAttributes -bor
             [Security.AccessControl.FileSystemRights]::WriteAttributes -bor
             [Security.AccessControl.FileSystemRights]::Delete -bor
             [Security.AccessControl.FileSystemRights]::DeleteSubdirectoriesAndFiles -bor
             [Security.AccessControl.FileSystemRights]::ChangePermissions -bor
             [Security.AccessControl.FileSystemRights]::TakeOwnership
    foreach ($rule in $acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier])) {
        if ($rule.AccessControlType -eq [Security.AccessControl.AccessControlType]::Allow -and
            ($rule.FileSystemRights -band $write) -ne 0 -and
            $rule.IdentityReference.Value -notin @('S-1-5-18','S-1-5-32-544')) { throw 'untrusted_write_acl' }
    }
}
function Ensure-PrivateDirectory([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) {
        $acl = New-Object Security.AccessControl.DirectorySecurity
        $acl.SetSecurityDescriptorSddlForm('O:SYG:SYD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)')
        $null = [IO.Directory]::CreateDirectory($Path, $acl)
    }
    # Existing directories/ACLs are inspected, never repaired or taken over.
    Assert-Private $Path $true
}
function Open-Verified([string]$Path, [long]$Size, [string]$Hash) {
    Assert-Private $Path $false
    $stream = [IO.File]::Open($Path,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read)
    try {
        if ($stream.Length -ne $Size) { throw 'existing_file_size_conflict' }
        $hasher = [Security.Cryptography.SHA256]::Create()
        try { $actual = ([BitConverter]::ToString($hasher.ComputeHash($stream))).Replace('-','').ToLowerInvariant() }
        finally { $hasher.Dispose() }
        if ($actual -cne $Hash) { throw 'existing_file_hash_conflict' }
        $stream.Position = 0
        return ,$stream
    } catch { $stream.Dispose(); throw }
}
function Create-NewBytes([string]$Path, [byte[]]$Bytes) {
    $stream = [IO.File]::Open($Path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
    try { $stream.Write($Bytes,0,$Bytes.Length); $stream.Flush($true) }
    finally { $stream.Dispose() }
}
try {
    if ([Security.Principal.WindowsIdentity]::GetCurrent().User.Value -ne 'S-1-5-18') { throw 'system_run_command_required' }
    if ($StageID -cnotmatch '^[a-f0-9]{32}$') { throw 'invalid_stage' }
    $programData = [Environment]::GetFolderPath([Environment+SpecialFolder]::CommonApplicationData)
    $driveRoot = [IO.Path]::GetPathRoot($programData)
    if ($driveRoot.Length -ne 3 -or -not [char]::IsLetter($driveRoot[0]) -or
        $driveRoot[1] -ne ':' -or [int][char]$driveRoot[2] -ne 92 -or
        -not [IO.Directory]::Exists($programData)) { throw 'invalid_programdata' }
    if (([IO.DriveInfo]::new($driveRoot)).DriveType -ne [IO.DriveType]::Fixed) { throw 'invalid_programdata' }
    Assert-NoReparse $programData
    $qaRoot = Join-Path $programData 'GodUseVPN-Ultra-QA'
    Ensure-PrivateDirectory $qaRoot
    $stage = Join-Path $qaRoot $StageID
    Ensure-PrivateDirectory $stage
    $manifestBytes = [Convert]::FromBase64String('__MANIFEST__')
    $manifestText = [Text.Encoding]::UTF8.GetString($manifestBytes)
    $manifest = $manifestText | ConvertFrom-Json
    if ($manifest.version -ne 1 -or $manifest.kind -cne 'GodUseVPN-Ultra-QA-stage' -or
        $manifest.sha256 -cne $FixedZipHash -or $manifest.chunkBytes -ne 2097152 -or
        @($manifest.parts).Count -ne $Count -or $Count -lt 1 -or $Count -gt 256) { throw 'invalid_manifest' }
    $sum = [long]0
    for ($j = 0; $j -lt $Count; $j++) {
        $part = $manifest.parts[$j]
        if ($part.size -lt 1 -or $part.size -gt 2097152 -or $part.sha256 -cnotmatch '^[a-f0-9]{64}$' -or
            ($j -lt $Count - 1 -and $part.size -ne 2097152)) { throw 'invalid_manifest_part' }
        $sum += [long]$part.size
    }
    if ($sum -ne $manifest.size) { throw 'invalid_manifest_size' }
    $manifestPath = Join-Path $stage 'stage.json'
    $mh = [Security.Cryptography.SHA256]::Create()
    try { $manifestHash = ([BitConverter]::ToString($mh.ComputeHash($manifestBytes))).Replace('-','').ToLowerInvariant() }
    finally { $mh.Dispose() }
    if (-not (Test-Path -LiteralPath $manifestPath)) { Create-NewBytes $manifestPath $manifestBytes }
    $verifiedManifest = Open-Verified $manifestPath $manifestBytes.Length $manifestHash
    $verifiedManifest.Dispose()
__BODY__
} catch {
    $status = [string]$_.Exception.Message
    $known = @('reparse_rejected','wrong_object_type','untrusted_owner','directory_acl_not_private',
        'untrusted_write_acl','existing_file_size_conflict','existing_file_hash_conflict',
        'system_run_command_required','invalid_stage','invalid_programdata','invalid_manifest',
        'invalid_manifest_part','invalid_manifest_size','invalid_part_index',
        'payload_size_mismatch','payload_hash_mismatch','remote_io_failure')
    if ($status -cnotin $known) { $status = 'remote_io_failure' }
    Reply $false $status
    exit 1
}
"""


PART_BODY = r"""
    if ($Index -lt 0 -or $Index -ge $Count) { throw 'invalid_part_index' }
    $data = [Convert]::FromBase64String('__DATA__')
    $part = $manifest.parts[$Index]
    if ($data.Length -ne $part.size -or $data.Length -gt 2097152) { throw 'payload_size_mismatch' }
    $hasher = [Security.Cryptography.SHA256]::Create()
    try { $hash = ([BitConverter]::ToString($hasher.ComputeHash($data))).Replace('-','').ToLowerInvariant() }
    finally { $hasher.Dispose() }
    if ($hash -cne $part.sha256) { throw 'payload_hash_mismatch' }
    $partPath = Join-Path $stage ('part-{0:D4}.bin' -f $Index)
    $status = 'present'
    if (-not (Test-Path -LiteralPath $partPath)) {
        Create-NewBytes $partPath $data
        $status = 'written'
    }
    $verifiedPart = Open-Verified $partPath $part.size $part.sha256
    $verifiedPart.Dispose()
    Reply $true $status
"""


MERGE_BODY = r"""
    $zipPath = Join-Path $stage 'preview.zip'
    if (Test-Path -LiteralPath $zipPath) {
        $existingZip = Open-Verified $zipPath $manifest.size $FixedZipHash
        $existingZip.Dispose()
        Reply $true 'present'
    } else {
        # A failed merge is preserved; a subsequent run uses a fresh temp name.
        $mergePath = Join-Path $stage 'merge-__MERGE_NONCE__.tmp'
        $output = [IO.File]::Open($mergePath,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
        try {
            for ($j = 0; $j -lt $Count; $j++) {
                $partPath = Join-Path $stage ('part-{0:D4}.bin' -f $j)
                $part = $manifest.parts[$j]
                $input = Open-Verified $partPath $part.size $part.sha256
                try { $input.CopyTo($output,1048576) } finally { $input.Dispose() }
            }
            $output.Flush($true)
        } finally { $output.Dispose() }
        $verifiedZip = Open-Verified $mergePath $manifest.size $FixedZipHash
        $verifiedZip.Dispose()
        # .NET Framework's two-argument Move never overwrites an existing target.
        [IO.File]::Move($mergePath,$zipPath)
        $finalZip = Open-Verified $zipPath $manifest.size $FixedZipHash
        $finalZip.Dispose()
        Reply $true 'merged'
    }
"""


def build_script(manifest: dict, stage_id: str, nonce: str, index: int,
                 block: bytes | None) -> str:
    if not re.fullmatch(r"[a-f0-9]{32}", stage_id) or not re.fullmatch(r"[a-f0-9]{32}", nonce):
        raise StageError("invalid_script_identity")
    if manifest.get("sha256") != EXPECTED_ZIP_SHA256:
        raise StageError("fixed_package_hash_required")
    count = len(manifest["parts"])
    if not 1 <= count <= MAX_PARTS:
        raise StageError("invalid_part_count")
    if block is not None:
        if not 0 <= index < count or not 0 < len(block) <= CHUNK_BYTES:
            raise StageError("invalid_local_part")
        if len(block) != manifest["parts"][index]["size"] or hashlib.sha256(block).hexdigest() != manifest["parts"][index]["sha256"]:
            raise StageError("local_package_changed_stop")
        body = PART_BODY.replace("__DATA__", base64.b64encode(block).decode("ascii"))
        operation = "part"
    else:
        if index != -1:
            raise StageError("invalid_merge_index")
        body = MERGE_BODY.replace("__MERGE_NONCE__", uuid.uuid4().hex)
        operation = "merge"
    replacements = {"__NONCE__": nonce, "__OP__": operation, "__INDEX__": str(index),
                    "__COUNT__": str(count), "__STAGE__": stage_id,
                    "__MANIFEST__": base64.b64encode(compact_json(manifest).encode("ascii")).decode("ascii"),
                    "__BODY__": body}
    script = REMOTE_COMMON
    for marker, value in replacements.items():
        script = script.replace(marker, value)
    # Both whole-file and line-split representations fit, whichever CLI uses.
    for lines in ([script], script.splitlines()):
        request_size = len(compact_json({"commandId": "RunPowerShellScript", "script": lines}).encode("ascii"))
        if request_size >= MAX_REQUEST_BYTES:
            raise StageError("generated_request_too_large")
    return script


def parse_receipt(stdout: str, nonce: str, operation: str, index: int,
                  count: int) -> str:
    try:
        data = json.loads(stdout)
        records = data["value"]
        if not isinstance(records, list):
            raise ValueError()
        receipts = []
        marker = RECEIPT_PREFIX + nonce + ":"
        for entry in records:
            message = entry.get("message", "")
            if not isinstance(message, str):
                raise ValueError()
            if "stderr" in str(entry.get("code", "")).lower() and message.strip():
                raise StageError("remote_stderr_stop")
            for line in message.splitlines():
                if line.startswith(marker):
                    receipts.append(json.loads(line[len(marker):]))
        if len(receipts) != 1:
            raise ValueError()
        value = receipts[0]
        if (value.get("v") != 1 or value.get("op") != operation or
                value.get("index") != index or value.get("count") != count):
            raise ValueError()
        if value.get("ok") is not True or value.get("sha256Match") is not True:
            code = value.get("status", "remote_rejected")
            if not isinstance(code, str) or code not in REMOTE_ERROR_CODES:
                code = "remote_rejected"
            raise StageError(code)
        allowed = {"written", "present"} if operation == "part" else {"merged", "present"}
        if value.get("status") not in allowed:
            raise ValueError()
        return value["status"]
    except StageError:
        raise
    except (ValueError, KeyError, TypeError, AttributeError):
        raise StageError("remote_receipt_missing_or_invalid_outcome_unknown") from None


def invoke_az(script: str, target: dict, nonce: str, operation: str,
              index: int, count: int, *, runner=subprocess.run, executable=None) -> str:
    az = executable or shutil.which("az")
    if not az:
        raise StageError("azure_cli_not_available")
    # A private local temporary directory holds @file so the base64 payload never
    # appears in command argv, shell history or our terminal. Always remove only
    # this locally created temporary object; no remote cleanup is attempted.
    with tempfile.TemporaryDirectory(prefix="ultra-qa-stage-") as temporary:
        script_path = Path(temporary) / "command.ps1"
        script_path.write_text(script, encoding="ascii", newline="\n")
        arguments = [az, "vm", "run-command", "invoke", "--subscription", target["subscription"],
                     "--resource-group", target["resource_group"], "--name", target["vm_name"],
                     "--command-id", "RunPowerShellScript", "--scripts", "@" + str(script_path),
                     "--only-show-errors", "--output", "json"]
        environment = os.environ.copy()
        environment["AZURE_CORE_COLLECT_TELEMETRY"] = "no"
        environment["AZURE_CORE_NO_COLOR"] = "true"
        try:
            result = runner(arguments, shell=False, capture_output=True, text=True,
                            encoding="utf-8", errors="replace", timeout=CALL_TIMEOUT_SECONDS,
                            env=environment, check=False)
        except subprocess.TimeoutExpired:
            raise StageError("azure_timeout_remote_outcome_unknown_wait_before_resume") from None
        except OSError:
            raise StageError("azure_cli_launch_failed") from None
        if result.returncode != 0:
            raise StageError("azure_command_failed_raw_output_suppressed")
        return parse_receipt(result.stdout, nonce, operation, index, count)


def stage_package(package: Path, target: dict, dry_run: bool) -> None:
    try:
        flags = os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0) | getattr(os, "O_NONBLOCK", 0)
        descriptor = os.open(package, flags)
        with os.fdopen(descriptor, "rb") as stream:
            details = os.fstat(stream.fileno())
            if not stat.S_ISREG(details.st_mode):
                raise StageError("local_zip_must_be_regular_file")
            manifest = inspect_stream(stream, EXPECTED_ZIP_SHA256)
            print("LOCAL zip sha256=match", flush=True)
            count = len(manifest["parts"])
            print(f"STATE parts={count} stage-id={target['stage_id']}", flush=True)
            stream.seek(0)
            for index, part in enumerate(manifest["parts"]):
                block = stream.read(part["size"])
                nonce = uuid.uuid4().hex
                script = build_script(manifest, target["stage_id"], nonce, index, block)
                if dry_run:
                    print(f"LOCAL part={index + 1}/{count} request-size=within-limit", flush=True)
                else:
                    status = invoke_az(script, target, nonce, "part", index, count)
                    print(f"PART {index + 1}/{count} status={status} sha256=match", flush=True)
            if stream.read(1):
                raise StageError("local_package_changed_stop")
            nonce = uuid.uuid4().hex
            script = build_script(manifest, target["stage_id"], nonce, -1, None)
            if dry_run:
                print("LOCAL merge request-size=within-limit; no remote calls", flush=True)
            else:
                status = invoke_az(script, target, nonce, "merge", -1, count)
                print(f"PACKAGE status={status} sha256=match; not installed or executed", flush=True)
    except OSError:
        raise StageError("local_package_read_failed") from None


def self_test() -> None:
    """Only generated text, synthetic receipts, a fake runner and local tempfiles."""
    import contextlib

    target = validate_target("11111111-1111-4111-8111-111111111111", "test-rg", "test-vm",
                             "11111111111141118111111111111111")
    for key, bad in [("subscription", "$(id)"), ("resource_group", "rg;id"),
                     ("vm_name", "--help"), ("stage_id", "../unsafe")]:
        values = dict(target)
        values[key] = bad
        try:
            validate_target(**values)
        except StageError:
            pass
        else:
            raise AssertionError("unsafe target accepted")
    payload = b"x" * CHUNK_BYTES
    generated = inspect_stream(io.BytesIO(payload), hashlib.sha256(payload).hexdigest())
    # build_script accepts only the fixed deployment hash, not arbitrary ZIPs.
    try:
        build_script(generated, target["stage_id"], uuid.uuid4().hex, 0, payload)
    except StageError:
        pass
    else:
        raise AssertionError("fixed deployment hash bypass")
    generated["sha256"] = EXPECTED_ZIP_SHA256
    nonce = uuid.uuid4().hex
    script = build_script(generated, target["stage_id"], nonce, 0, payload)
    assert "GodUseVPN-Ultra-QA" in script and "CreateNew" in script
    assert "CreateDirectory($Path, $acl)" in script
    assert "Expand-Archive" not in script and "Start-Process" not in script
    assert len(compact_json({"commandId": "RunPowerShellScript", "script": [script]})) < MAX_REQUEST_BYTES
    receipt = {"v": 1, "ok": True, "op": "part", "index": 0, "count": 1,
               "status": "written", "sha256Match": True}
    good = compact_json({"value": [{"code": "ComponentStatus/StdOut/succeeded",
                                    "message": RECEIPT_PREFIX + nonce + ":" + compact_json(receipt)}]})
    assert parse_receipt(good, nonce, "part", 0, 1) == "written"
    failed = dict(receipt, ok=False, sha256Match=False, status="PRIVATE_VALUE")
    failed_response = compact_json({"value": [{"code": "ComponentStatus/StdOut/failed",
                                               "message": RECEIPT_PREFIX + nonce + ":" + compact_json(failed)}]})
    for bad in ("not json", good.replace(nonce, uuid.uuid4().hex), failed_response,
                compact_json({"value": [{"code": "StdErr", "message": "PRIVATE_VALUE"}]})):
        try:
            parse_receipt(bad, nonce, "part", 0, 1)
        except StageError as error:
            assert "PRIVATE_VALUE" not in str(error)
        else:
            raise AssertionError("invalid remote receipt accepted")
    calls = []
    def fake_run(arguments, **options):
        assert options["shell"] is False and options["capture_output"] is True
        assert arguments[arguments.index("--scripts") + 1].startswith("@")
        path = Path(arguments[arguments.index("--scripts") + 1][1:])
        assert path.read_text("ascii") == script
        calls.append(arguments)
        return subprocess.CompletedProcess(arguments, 0, good, "RAW_PRIVATE_STDERR")
    terminal = io.StringIO()
    with contextlib.redirect_stdout(terminal), contextlib.redirect_stderr(terminal):
        assert invoke_az(script, target, nonce, "part", 0, 1,
                         runner=fake_run, executable="az") == "written"
    assert terminal.getvalue() == "" and len(calls) == 1
    assert not Path(calls[0][calls[0].index("--scripts") + 1][1:]).exists()
    merge = build_script(generated, target["stage_id"], uuid.uuid4().hex, -1, None)
    assert "[IO.File]::Move($mergePath,$zipPath)" in merge
    assert "Remove-Item" not in script + merge and "[IO.File]::Delete" not in script + merge
    print("LOCAL self-test=passed; no az, remote writes or network calls", flush=True)


def main(argv=None) -> int:
    parser = QuietParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--zip", type=Path, dest="package")
    parser.add_argument("--subscription")
    parser.add_argument("--resource-group")
    parser.add_argument("--vm-name")
    parser.add_argument("--stage-id", help="Fresh lowercase UUID4 hex; reuse only for this ZIP's resume")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--self-test", action="store_true")
    try:
        if sys.version_info < (3, 10):
            raise StageError("python_310_or_later_required")
        args = parser.parse_args(argv)
        if args.self_test:
            if args.package or args.subscription or args.resource_group or args.vm_name or args.stage_id or args.dry_run:
                raise StageError("self_test_takes_no_target")
            self_test()
            return 0
        if not all((args.package, args.subscription, args.resource_group, args.vm_name, args.stage_id)):
            raise StageError("explicit_zip_subscription_group_vm_stage_required")
        target = validate_target(args.subscription, args.resource_group, args.vm_name, args.stage_id)
        if not args.dry_run and os.name != "posix":
            raise StageError("run_manually_in_bash_cloud_shell")
        stage_package(args.package, target, args.dry_run)
        return 0
    except StageError as error:
        print("STOP status=" + str(error), file=sys.stderr, flush=True)
        print("STATE stopped; no automatic retry; wait for any prior Run Command to finish.", file=sys.stderr, flush=True)
        print("STATE resume with the same explicit target and stage-id; verified parts are reused.", file=sys.stderr, flush=True)
        print("STATE conflicting/incomplete objects are preserved; use a fresh stage-id or review manually.", file=sys.stderr, flush=True)
        return 1
    except KeyboardInterrupt:
        print("STOP status=interrupted_remote_outcome_unknown; wait before same-stage resume", file=sys.stderr, flush=True)
        return 130
    except Exception:
        # Never print an exception, traceback, argv or raw az response: those may
        # include subscription/VM identities, local paths or extension log data.
        print("STOP status=unexpected_local_failure; raw details suppressed", file=sys.stderr, flush=True)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
