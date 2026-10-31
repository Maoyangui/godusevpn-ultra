# GodUseVPN Ultra

GodUseVPN Ultra is an independent privacy workspace built on GodUseVPN for Windows desktops. It puts VPN routing, browser profiles, privacy settings, and readable evidence into one guided workflow.

Current preview: **v0.02 · Windows x64 · unsigned**.

## What it includes

- GodUseVPN subscriptions, nodes, TUN, DNS, routing rules, and connection flow.
- Supported Windows privacy policies with original-value backup and independent readback.
- Per-browser profiles for OS environment, User-Agent, GPU, viewport, and proxy.
- SOCKS5, HTTP, and HTTPS proxy settings with an explicit connectivity check.
- Privacy and evidence centers that show scope, source, readback time, unknown fields, and recovery records.
- JPEG / PNG metadata review and copy-out cleanup without overwriting the source file.
- Encrypted browser-profile archives with integrity checks and sensitive connection fields removed on export.
- Official Chrome / Firefox version catalogs and downloaded-engine management; unavailable runtimes fail closed.

The browser lab can preview Windows, macOS, Android, and iOS environments. Real browser windows currently use a Windows WebView2 host; other systems are configuration previews and are labelled accordingly.

## Download and install

1. Download `godusevpn-ultra-0.02-windows-amd64.zip` and its `.sha256` file from [Releases](https://github.com/Maoyangui/godusevpn-ultra/releases).
2. Verify SHA256 and extract the archive to a local fixed-disk folder.
3. In an elevated PowerShell, run `./Install-Ultra.ps1 -WhatIf` to review the target, then run `./Install-Ultra.ps1`.
4. Start `godusevpn.exe`, add your subscription, and enable the controls you need.
5. Refresh the evidence center after changes; open the selected browser profile and read back runtime values when required.

The package requires Microsoft WebView2 and does not download or install it automatically. This unsigned preview is intended for testing.

## How the workflow behaves

- Review scope before writing; a saved setting is not proof of runtime effect.
- Evidence is labelled verified, partial, review needed, or unknown. Unknown never becomes “protected”.
- Failures keep the original value, reason, and recovery path visible without changing VPN connection state.
- Browser profiles, cookies, and proxy drafts stay isolated. Archives use AES-256-GCM and never store panel or proxy secrets.
- Each browser keeps its own UA, GPU, viewport, and proxy draft, then reads the opened runtime back.

## Build

```powershell
go install github.com/tc-hib/go-winres@v0.3.3
./build-ultra.ps1 -Version 0.02
```

The build script runs regression tests, creates Windows resources, builds the client / service / CLI, and writes a lowercase package plus SHA256 file. Existing output folders are never overwritten.

## Verification status

| Area | Status |
| --- | --- |
| Showcase page, interaction motion, English / Chinese switching | Verified at 1440 / 1024 / 768 / 390 widths |
| Browser profile, proxy draft, engine removal, archive encryption | Automated regression passed |
| Real Windows VPN, proxy exit, WebRTC / WebGPU / GPS | Requires the corresponding VPS or device run |
| Windows 10 1809, 19044 / 22H2, Windows 11 x64 / ARM64 | Marked supported only after version evidence is recorded |

Unverified capabilities stay unknown; the showcase page is not used as runtime evidence.

## Restore and uninstall

Original values are saved before changes. Run `./Uninstall-Ultra.ps1 -WhatIf` first, then execute the uninstall script as administrator. Failed recovery keeps the tools and records available for follow-up.

See the [project plan](PROJECT_PLAN.zh-CN.md), [feature ledger](docs/ultra/FEATURE_STATUS.zh-CN.md), and [recovery guide](docs/ultra/INSTALL_RECOVERY.en.md) for detailed records.

## Links

- [Ultra showcase](https://maoyangui.github.io/godusevpn-ultra/)
- [Node subscription](https://godusevpn.org/subscription/)
- [Browser companion](browser-extension/README.md)
- [中文 README](README-ULTRA.zh-CN.md)

Licensed under GPL-3.0. Third-party notices are in [NOTICE](NOTICE).
