<p align="center"><img src="docs/ultra/assets/ultra-logo-square.svg" width="120" alt="GodUseVPN Ultra"></p>

# GodUseVPN Ultra

A Windows privacy companion built on [GodUseVPN](https://github.com/Maoyangui/godusevpn), retaining its sing-box VPN client and adding separate privacy controls and a local detection center.

**v0.01 beta · Windows x64 · unsigned · experimental.** A saved policy or installed filter is configuration evidence, not proof that every app or website is blocked. The [feature ledger](docs/ultra/FEATURE_STATUS.zh-CN.md) and [audit ledger](docs/ultra/AUDIT_RECHECK_SUMMARY.zh-CN.md) separate source review, simulated tests and real Windows tests.

Development continues against the [complete H01–H35 requirements](docs/ultra/HANDOVER_REQUIREMENTS_2026-10-26.zh-CN.md). Earlier beta batches do not cancel later browser, GPS, open source integration or UI requirements. Source and simulated tests do not satisfy pending Windows acceptance; the first section of the [feature ledger](docs/ultra/FEATURE_STATUS.zh-CN.md) records the current state.

[Downloads](https://github.com/Maoyangui/godusevpn-ultra/releases) · [中文说明](README-ULTRA.zh-CN.md) · [Project plan](PROJECT_PLAN.zh-CN.md) · [Browser companion](browser-extension/README.md) · [Recovery guide](docs/ultra/INSTALL_RECOVERY.en.md)

## Included features

| Area | Controls | Scope |
|---|---|---|
| VPN | Subscriptions, nodes, TUN, routing, DNS, modes and the existing global no-direct guard | Requires your subscription and the separately installed Ultra service |
| Windows policies | Advertising ID, input personalization, clipboard history and cloud sync | Four fixed policies; OS support checked before writing |
| Browser policies | WebRTC non-proxied UDP, prediction, suggestions, diagnostics, location, camera, microphone, notifications, sensors, third-party cookies and WebGL | Eleven policies each for Chrome and Edge, in the original user's context; browser acceptance requires verification |
| Time zone | Enumerated zones, explicit apply, backup and conflict-aware restore | Preserves daylight-saving preference; does not change UTC or automatically match an exit IP |
| Application rules | Restrict a selected executable's network, private/link-local ranges, multicast or IPv4 limited broadcast | Separate WFP objects; no localhost isolation or guarantee for proxy relay |
| Detection center | Current settings, evidence source/time, observer identity, helper state, remaining readable information and suggested actions | Local bounded observations; unknown stays unknown |
| Browser companion | Supported privacy/content APIs, per-API choices, control checks, restore, local environment tests and manual tracking-parameter cleanup | Separate per profile; no all-sites read permission or background telemetry |
| Data tools | Selected JPEG/PNG metadata inspection and removal to a copy; reviewed detection and diagnostic exports | No overwrite; limited formats and metadata blocks; sensitive diagnostics require explicit selection |
| Independent browser | WebView2 profile editor, four emulated OS environments, region, viewport, WebGL vendor/renderer and a per-browser proxy | Ordinary-user host; still requires the Ultra strict tunnel; real runtime acceptance pending |
| Browser versions | Official Chrome for Testing and Firefox catalogs, bounded archive verification and a controlled Windows native launch path | Real browser, proxy and traffic behavior still requires VM evidence; unsupported platforms remain fail-closed |
| EXIF component | Pinned complete ExifTool 13.59 distribution; seven selected EXIF groups in JPEG/PNG | Values require explicit reveal; copy only; XMP/IPTC/thumbnails remain outside scope; VM execution pending |
| Secret clipboard | Optional 5–600 second clearing for a secret copied by Ultra | Default off; sequence, owner and content must still match; history/cloud/other readers remain outside scope |

Privacy controls and findings have small `?` explanations, explicit actions, and light/dark layouts. Privacy and detection pages work without a subscription. Most new privacy text currently uses Chinese; this English README does not imply complete English UI coverage.

## Install and use

Use Windows x64, Microsoft WebView2 and **64-bit PowerShell**. Only Windows x64 is packaged for this beta. Upstream macOS/Linux/Android scripts and installers are not Ultra releases.

1. Download `godusevpn-ultra-0.01-beta-windows-amd64.zip` and its `.sha256` from this repository's Releases. Verify the hash and fully extract.
2. Run `godusevpn.exe` to inspect the interface. The GUI stays in the original account. Startup requests a separate headless administrator helper; cancellation leaves the interface available. `--ultra-no-elevate` skips this request for inspection.
3. To install the VPN service, run `./Install-Ultra.ps1 -WhatIf` first. Then execute `./Install-Ultra.ps1` elevated. It supports **fresh installation only**, into `%ProgramFiles%\GodUseVPN Ultra`. Existing/partial installations are preserved. Follow the [recovery guide](docs/ultra/INSTALL_RECOVERY.en.md) if it stops.
4. Start the installed client, add your subscription and choose the controls you need. Adding or refreshing a subscription contacts its provider. This package includes no subscription and does not automatically download WebView2.
5. Refresh detection after changing settings. Install the optional extension manually in each intended browser profile and inspect that browser's policy/permission pages.

Binaries are unsigned. Checksums establish transfer integrity, not publisher identity. Review the source and validation record before granting administrator access.

File tools and report saving currently accept ordinary subdirectories on verified local fixed disks. USB/removable drives, mapped or redirected paths and saving directly to a volume root are unsupported; choose a local subfolder.

## Architecture and failure behavior

The GUI runs as the original user. A fixed-operation helper handles authorized machine changes; it accepts no arbitrary commands, scripts or registry paths. Current-user policies are not redirected into an alternate administrator account. Write-ahead records, readback and compare-before-restore preserve original values and external conflicts.

Optional privacy failures do not call VPN connect/disconnect or remove its guard. The existing guard retains its own failure rules. Unverifiable service identity is shown as unknown; it does not trigger silent service replacement or network recovery. Official machine cleanup and uninstall share one maintenance gate and preserve tools/records on failure.

Ultra has independent service/data/IPC/protocol/startup/TUN/WFP identities and local ports 12080/19090 by default. Run only one whole-device VPN tunnel at a time: IPv6 bindings, routes and WFP arbitration remain shared resources. Upstream update installation is disabled until an independently verified Ultra update channel exists.

## Privacy boundaries

- Websites may still read their accounts/cookies, the IP visible to that route, time zone, languages, screen/graphics capabilities, fonts and other fingerprint signals. Restricting non-proxied WebRTC UDP is not complete WebRTC removal. A WebGL policy does not prove WebGPU is disabled.
- Native apps can often read more than websites. Administrator rights do not universally hide hardware identifiers, GPS sources, adapters or all read operations. This beta has no global hardware noise, kernel sandbox or complete port-scan prevention.
- Filter existence does not prove blocking. The tunnel's `permitSelf` hard permit, other filters, existing connections and proxy relay require real traffic checks. Localhost is explicitly outside these application rules.
- Detection does not automatically contact external test sites, send STUN, scan the LAN or collect raw MACs, serials, cookies or tokens. Existing VPN operations still include subscription refresh, node tests and exit queries; separate per-purpose controls remain roadmap work.
- Detection exports omit environment/observer details by default and save the reviewed snapshot. Default diagnostics contain a minimal status summary. Opted-in logs/configuration/crash/network details may remain identifying despite common credential redaction; review before sharing.
- Abnormal/legacy machine recovery stores are rejected and preserved. A same-account concurrent path-redirection limitation in optional user-store writes remains documented as **C04** in the [audit ledger](docs/ultra/AUDIT_RECHECK_SUMMARY.zh-CN.md); it is not marked fully resolved.

First-network prompts, destination allowlists, general DPAPI credential migration, global anti-fingerprinting and automatic language/device-name changes remain unfinished [requirements](docs/ultra/FEATURE_STATUS.zh-CN.md). Isolated browsers and timed secret clipboard clearing have source implementations above; their real acceptance is still pending. Browser OS emulation does not run native Safari/WebKit or change the host OS.

## Restore and uninstall

Restore browser policies and time zone in **each original user's account** before uninstalling. Conflicting external changes are preserved. The CLI provides `privacy timezone-report` and `privacy timezone-restore`; SYSTEM machine cleanup does not restore another user's HKCU.

Close Ultra windows/helpers. Preview `./Uninstall-Ultra.ps1 -WhatIf`, then run `./Uninstall-Ultra.ps1` elevated from the fixed installation directory. It restores/verifies machine privacy policies and application filters before removing the Ultra service. Failures preserve recovery tools and records. Files and user data are retained for review: do not delete the tools first. See [detailed recovery](docs/ultra/INSTALL_RECOVERY.en.md).

## Build and verify

Use the Go version in `go.mod`, Node.js and PowerShell on Windows:

```powershell
go install github.com/tc-hib/go-winres@v0.3.3
./build-ultra.ps1 -Version 0.01-beta
node tools/audit-built-package.cjs dist/godusevpn-ultra-0.01-beta-windows-amd64
```

The script runs isolated regression tests, generates resources, builds GUI/CLI/service and creates a ZIP with checksums. It does not start the product, install a service or change network settings. Existing outputs are refused. Tests use fake registry/WFP or temporary fixtures; live tests are opt-in and excluded from CI. Development-only DOM/visual dependencies are pinned under `tools/qa`.

Tag: `v0.01-beta`; displayed release: **v0.01 beta**; numeric Windows file version: `0.1.0.0`. The Windows workflow checks source, tests, PE resources, package/ZIP inventory and embedded frontend. CI success alone does not complete release acceptance. Real UAC, managed browsers, reboot persistence and end-to-end network validation remain pending wherever the ledgers lack evidence; untested versions and scenarios are not certified.

## License and attribution

Based on [Maoyangui/godusevpn](https://github.com/Maoyangui/godusevpn), copied from commit `b51aed4`. The original project directory is kept unchanged. Copyright, [GPL-3.0](LICENSE), [NOTICE](NOTICE) and third-party notices remain applicable. The upstream Go module path is retained for source compatibility; this repository and its releases are the Ultra fork.
