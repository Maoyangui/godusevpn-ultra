#!/usr/bin/env python3
"""Python 3.10+, authenticated Bash Cloud Shell. Fixed expand OR smoke only.

Required explicit options: --subscription GUID --resource-group NAME --vm-name
NAME --stage-id UUID4_HEX --step expand|smoke. Run expand first and inspect its
success before separately selecting smoke. No login, parallelism or auto retry.
The matching, unchanged .ps1 must be beside this launcher. This launcher never
prints raw Azure responses, errors, target IDs or a traceback.

Guest script bytes are gzip decoded and hash checked before ScriptBlock.Create.
A fresh start tag binds stdout to this invocation, but is NOT a completion
receipt: only the pinned script's unique, validated result proves completion.
The pinned script may exit the host, so no post-execution nonce is promised.
Expand writes only its three specified payload files; smoke runs only CLI
version/privacy report, with SYSTEM scope, not GUI or restoration verification.
"""

from __future__ import annotations

import argparse
import base64
import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import uuid

SCRIPTS = {
    "expand": ("Expand-VpsCliSmoke.ps1", "afb7afa9d271f6a24ca6a1063e75047b4dc66a30812af64126d6c91631ebda75"),
    "smoke": ("Vps-ReadOnlySmoke.ps1", "f7a4254a05078ba9bd7c8b88e9178c732661ed5e5a08d69d33fc2d0e08d3636a"),
}
CLI_SHA256 = "bf728ab1a000b4a8aae5329d7ba76cc25aae4c7a87667fecc0bfdf4d9f5ffaa1"
PREFIX = "ULTRA_QA_START:"
LIMITS = ["current_token_not_GUI_user", "no_service_or_helper_contact",
          "no_detection_or_network_test", "configuration_not_runtime_protection",
          "hash_not_package_signature", "no_report_or_error_text_exported",
          "trusted_stage_and_ancestor_ACLs_verified_externally", "leaf_handle_does_not_lock_parent_namespace"]
STATUS_KEYS = {"available", "manual", "configured", "policy_conflict", "unknown", "recovery_pending",
               "external_change", "unsupported", "requires_admin", "error", "other"}
READ_KEYS = {"not_checked", "different", "match", "machine_override", "unknown", "other"}

WRAPPER = r"""
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$WarningPreference = 'SilentlyContinue'
Set-StrictMode -Version 2
[Console]::Out.WriteLine('ULTRA_QA_START:__NONCE__:__STEP__')
try {
    if ([Security.Principal.WindowsIdentity]::GetCurrent().User.Value -ne 'S-1-5-18') { throw 'system_required' }
    $compressed = [Convert]::FromBase64String('__GZIP__')
    $inputStream = New-Object IO.MemoryStream(,$compressed)
    $outputStream = New-Object IO.MemoryStream
    $gzipStream = New-Object IO.Compression.GZipStream($inputStream,[IO.Compression.CompressionMode]::Decompress)
    try { $gzipStream.CopyTo($outputStream); $bytes = $outputStream.ToArray() }
    finally { $gzipStream.Dispose(); $inputStream.Dispose(); $outputStream.Dispose() }
    if ($bytes.Length -ne __LENGTH__ -or $bytes.Length -gt 65536) { throw 'script_size_mismatch' }
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $actual = ([BitConverter]::ToString($sha.ComputeHash($bytes))).Replace('-','').ToLowerInvariant() }
    finally { $sha.Dispose() }
    if ($actual -cne '__HASH__') { throw 'script_hash_mismatch' }
    $encoding = New-Object Text.UTF8Encoding($false,$true)
    $body = [ScriptBlock]::Create($encoding.GetString($bytes))
__CALL__
} catch {
    [Console]::Out.WriteLine('{"launcherVersion":1,"status":"wrapper_failed"}')
    exit 1
}
"""


class QaError(Exception):
    pass


class Parser(argparse.ArgumentParser):
    def error(self, message):
        raise QaError("invalid_arguments_use_help")


def target_values(subscription, resource_group, vm_name, stage_id):
    try:
        sub = uuid.UUID(subscription)
        stage = uuid.UUID(hex=stage_id)
    except (ValueError, AttributeError):
        raise QaError("invalid_target_or_stage") from None
    if str(sub) != subscription.lower() or sub.int == 0:
        raise QaError("invalid_subscription_guid")
    if not re.fullmatch(r"[A-Za-z0-9_][A-Za-z0-9_.()-]{0,89}", resource_group) or resource_group.endswith("."):
        raise QaError("invalid_resource_group")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,63}", vm_name) or vm_name.endswith((".", "-")):
        raise QaError("invalid_vm_name")
    if not re.fullmatch(r"[a-f0-9]{32}", stage_id) or stage.version != 4 or stage.variant != uuid.RFC_4122:
        raise QaError("invalid_stage_uuid4")
    return {"subscription": str(sub), "resource_group": resource_group, "vm_name": vm_name, "stage_id": stage_id}


def build_wrapper(step, stage_id, nonce):
    if step not in SCRIPTS or not re.fullmatch(r"[a-f0-9]{32}", stage_id) or not re.fullmatch(r"[a-f0-9]{32}", nonce):
        raise QaError("invalid_wrapper_inputs")
    name, expected = SCRIPTS[step]
    source = Path(__file__).resolve().with_name(name).read_bytes()
    if len(source) > 65536 or hashlib.sha256(source).hexdigest() != expected:
        raise QaError("local_script_hash_mismatch")
    if step == "expand":
        call = "    & $body -StageID '" + stage_id + "'"
    else:
        call = r"""    $pd = [Environment]::GetFolderPath([Environment+SpecialFolder]::CommonApplicationData)
    $directory = [IO.Path]::Combine($pd,'GodUseVPN-Ultra-QA','__STAGE__','cli-smoke')
    & $body -PackageDirectory $directory -ExpectedCliSha256 '__CLI_HASH__'"""
        call = call.replace("__STAGE__", stage_id).replace("__CLI_HASH__", CLI_SHA256)
    values = {"__NONCE__": nonce, "__STEP__": step, "__LENGTH__": str(len(source)), "__HASH__": expected,
              "__GZIP__": base64.b64encode(gzip.compress(source, mtime=0)).decode("ascii"), "__CALL__": call}
    script = WRAPPER
    for marker, value in values.items():
        script = script.replace(marker, value)
    return script


def require(condition):
    if not condition:
        raise QaError("remote_result_invalid_or_failed")


def exact(value, keys):
    require(isinstance(value, dict) and set(value) == set(keys))


def bounded(value, maximum=256):
    require(type(value) is int and 0 <= value <= maximum)


def validate_result(value, step):
    require(isinstance(value, dict) and value.get("status") == "completed" and value.get("stage") == "complete")
    require(type(value.get("schemaVersion")) is int and value["schemaVersion"] == 1)
    if step == "expand":
        expected = {"schemaVersion": 1, "status": "completed", "stage": "complete", "observer": "SYSTEM",
                    "zipSha256Match": True, "entryCountVerified": 32, "checksumsVerified": 30,
                    "extractedFileCount": 3, "cliSha256Match": True, "smokeReady": True,
                    "outputLeaf": "cli-smoke", "productExecuted": False}
        exact(value, expected)
        require(all(type(value[key]) is type(want) and value[key] == want for key, want in expected.items()))
        return value
    exact(value, {"schemaVersion", "mode", "status", "stage", "cliHashMatched", "localPathsChecked",
                  "token", "version", "report", "limits"})
    require(value["mode"] == "hash_pinned_CLI_version_and_privacy_report_only")
    require(value["cliHashMatched"] is True and value["localPathsChecked"] is True and value["limits"] == LIMITS)
    require(isinstance(value["version"], str) and re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9.+_-]{0,79}", value["version"]))
    token = value["token"]
    exact(token, {"kind", "elevated", "interactiveSid", "userScope", "helperStarted"})
    require(token["kind"] == "SYSTEM" and token["elevated"] is True and token["helperStarted"] is False)
    require(type(token["interactiveSid"]) is bool and token["userScope"] == "current_token_HKCU_and_APPDATA_only_not_GUI_user")
    report = value["report"]
    exact(report, {"platform", "observerElevated", "observerAdmin", "backupHealthyForCurrentTokenUserJournal",
                   "expectedPolicyCount", "recognizedPolicyCount", "missingPolicyCount", "guidanceOrOtherCount",
                   "managedCount", "canApplyCount", "statusCounts", "readStatusCounts", "runtimeProtection",
                   "machineBackupInspection", "browserAcceptance"})
    require(report["platform"] == "windows" and report["runtimeProtection"] == "not_tested" and
            report["machineBackupInspection"] == "not_performed" and report["browserAcceptance"] == "not_tested")
    for key in ("observerElevated", "observerAdmin", "backupHealthyForCurrentTokenUserJournal"):
        require(type(report[key]) is bool)
    for key in ("expectedPolicyCount", "recognizedPolicyCount", "missingPolicyCount", "guidanceOrOtherCount", "managedCount", "canApplyCount"):
        bounded(report[key])
    require(report["expectedPolicyCount"] == 26 and report["recognizedPolicyCount"] + report["missingPolicyCount"] == 26)
    require(report["managedCount"] <= report["recognizedPolicyCount"] and report["canApplyCount"] <= report["recognizedPolicyCount"])
    for key, allowed in (("statusCounts", STATUS_KEYS), ("readStatusCounts", READ_KEYS)):
        exact(report[key], allowed)
        for number in report[key].values():
            bounded(number)
        require(sum(report[key].values()) == report["recognizedPolicyCount"])
    return value


def unique_json(text):
    def pairs(items):
        value = {}
        for key, item in items:
            if key in value:
                raise ValueError()
            value[key] = item
        return value
    return json.loads(text, object_pairs_hook=pairs)


def parse_response(raw, step, nonce):
    try:
        require(len(raw) <= 65536)
        response = unique_json(raw)
        require(isinstance(response, dict) and isinstance(response.get("value"), list))
        outputs = []
        for item in response["value"]:
            require(isinstance(item, dict) and isinstance(item.get("message"), str))
            code = item.get("code", "")
            require(isinstance(code, str))
            if "/StdErr/" in code:
                require(not item["message"].strip())
            elif "/StdOut/" in code:
                outputs.append(item["message"])
        require(len(outputs) == 1)
        lines = outputs[0].strip().splitlines()
        # Recognize only Azure's empty stream framing, not arbitrary extra text.
        if lines and lines[0].strip() == "[stdout]":
            lines.pop(0)
        if lines and lines[-1].strip() == "[stderr]":
            lines.pop()
        require(bool(lines) and lines[0] == PREFIX + nonce + ":" + step)
        value = unique_json("\n".join(lines[1:]).strip())
        return validate_result(value, step)
    except QaError:
        raise
    except (ValueError, TypeError, KeyError, AttributeError):
        raise QaError("remote_response_invalid_or_unknown") from None


def invoke(step, target, *, runner=subprocess.run, executable=None):
    nonce = uuid.uuid4().hex
    script = build_wrapper(step, target["stage_id"], nonce)
    az = executable or shutil.which("az")
    if not az:
        raise QaError("azure_cli_unavailable")
    with tempfile.TemporaryDirectory(prefix="ultra-readonly-qa-") as temporary:
        path = Path(temporary) / "command.ps1"
        descriptor = os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
        with os.fdopen(descriptor, "w", encoding="ascii", newline="\n") as stream:
            stream.write(script)
        arguments = [az, "vm", "run-command", "invoke", "--subscription", target["subscription"],
                     "--resource-group", target["resource_group"], "--name", target["vm_name"],
                     "--command-id", "RunPowerShellScript", "--scripts", "@" + str(path),
                     "--only-show-errors", "--output", "json"]
        environment = os.environ.copy()
        environment["AZURE_CORE_COLLECT_TELEMETRY"] = "no"
        environment["AZURE_CORE_NO_COLOR"] = "true"
        try:
            result = runner(arguments, shell=False, capture_output=True, text=True, encoding="utf-8",
                            errors="replace", timeout=900, check=False, env=environment)
        except subprocess.TimeoutExpired:
            raise QaError("timeout_remote_outcome_unknown") from None
        except OSError:
            raise QaError("azure_cli_launch_failed") from None
        if result.returncode != 0:
            raise QaError("azure_command_failed_or_unknown")
        return parse_response(result.stdout, step, nonce)


def main(argv=None):
    parser = Parser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    for name in ("subscription", "resource-group", "vm-name", "stage-id"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--step", choices=tuple(SCRIPTS), required=True)
    try:
        if sys.version_info < (3, 10) or os.name != "posix":
            raise QaError("python310_bash_cloud_shell_required")
        options = parser.parse_args(argv)
        target = target_values(options.subscription, options.resource_group, options.vm_name, options.stage_id)
        result = invoke(options.step, target)
        print(json.dumps({"step": options.step, "status": "completed", "result": result}, separators=(",", ":")), flush=True)
        return 0
    except KeyboardInterrupt:
        code = "interrupted_remote_outcome_unknown"
    except QaError as error:
        code = str(error)
    except Exception:
        code = "local_failure_details_suppressed"
    print("STOP status=" + code + "; no automatic retry; wait and manually confirm remote outcome before another invocation", file=sys.stderr, flush=True)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
