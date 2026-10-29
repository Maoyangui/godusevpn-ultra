# Independent Windows browser host — source/compile stage

This directory contains a separate .NET Framework 4.6.2 x64 WinForms/WebView2
host. It has a real native window, Chinese address/navigation controls,
configuration/GPU status, and a WebView2 initialization path.
It does not use the existing Wails UI, expose a Go/COM host object, install a
service, or change the Go build. It is **not a completed privacy acceptance
result**. The host has not been executed in this work; all browser/runtime,
Windows pipe, proxy, CDP and traffic behavior remains pending controlled VM
testing. No browser/WebView2 was launched by the build or pure tests.

## Official pinned dependency and build

The SDK is the stable Microsoft.Web.WebView2 **1.0.4314.35**, published on
2026-10-23. Source: [official NuGet package](https://www.nuget.org/packages/Microsoft.Web.WebView2/1.0.4314.35).
The exact `.nupkg` comes from NuGet's official flat-container endpoint; its
SHA256 is `90778deb1b77e36292828b4b5833c9255c27533f40e0ddd55891ae718944edc4`.
The package's `LICENSE.txt` grants redistribution under Microsoft's included
three-clause BSD-style terms. `Build.ps1` copies that license beside the output.
The SDK/Core DLL's Authenticode signature was locally checked as Valid with a
Microsoft Corporation signer. This SDK package is a compile dependency, not
the separately installed WebView2 Runtime or its license.

Run from the project root:

```powershell
& .\browser-host\Build.ps1 -Version '0.01-beta' -RunPureTests
```

The script downloads only the pinned package when absent, verifies its complete
hash, extracts four fixed members, and invokes the existing Windows Framework64
`csc.exe`. No `dotnet`/NuGet/system installation is performed. Output and SDK
cache stay under `.codex/tmp/`. `-Version` defaults to `0.01-beta` and follows the
package build's numeric/prerelease syntax. The ignored generated AssemblyInfo
records that exact informational/product version and the four numeric Windows
file/assembly version components. The host window reads its own informational
version; packaging can inspect VERSIONINFO without executing the host.

The optional tests link no WebView2 or WinForms assembly. They use contract
strings, monotonic clock fixtures, gate state, synthetic TCP/security/native
call data, and a Node `vm` with mock WebGL/WebGPU objects for the fixed graphics
scripts. Node must be available on PATH for that pure script test. Tests do not
invoke WindowsGuard's native APIs, open a socket, or access a real GPU.
**The script never runs the browser host.**

Expected reviewed install layout for a future packaging step:

```text
%ProgramFiles%\GodUseVPN Ultra\
  godusevpn.exe
  godusevpn-svc.exe
  browser-host\
    GodUseVPN.Ultra.BrowserHost.exe
    GodUseVPN.Ultra.BrowserHost.exe.config
    Microsoft.Web.WebView2.Core.dll
    Microsoft.Web.WebView2.WinForms.dll
    WebView2Loader.dll
    Microsoft.Web.WebView2.LICENSE.txt
```

This script does not install/copy those files into Program Files. A development output
cannot be launched as a trusted installed host; the fixed-path check rejects it.

## Trusted caller contract

The ordinary-user installed GUI must start the fixed host directly with no
arguments, and redirect its stdin with a real anonymous Windows pipe created
by that same GUI process. The host verifies its actual token, fixed installed
parent image and same user, trusted code ACLs/ancestors, and the pipe's OS server
PID using `GetNamedPipeServerProcessId`. Failure is terminal; there is no
fallback to parent-name-only trust. Compatibility of Go `os.Pipe`/`StdinPipe`
and the Windows pipe PID API is explicitly a VM acceptance item.

The first UTF-8/ASCII JSON line, bounded to 32 KiB, is:

```json
{"schemaVersion":1,"servicePid":1234,"proxyPort":2080,"connectedStrictVerified":true,"profile":{}}
```

`profile` must be the complete `internal/browserhost.ProfileConfig` object,
with precisely the host-supported values below. Unknown/duplicate fields,
trailing JSON, paths, custom engine binaries and extra flags are rejected.
After the first line, send `{"type":"lease"}` every 500 ms. Three seconds
without a lease, EOF, `{"type":"close"}`, or any unknown/malformed frame
permanently closes the relay and window. A background watchdog enforces lease
expiry independently of the WinForms/COM event loop. No later frame reopens it.

The Go caller must continuously maintain fresh authenticated IPC evidence:
connected/wanted, global, NoDirect, StrictRoute, TUN, guard on, no guard error,
unchanged owned listener and installed SCM instance. It must close stdin on
any failure or unknown state. `connectedStrictVerified` is an attestation from
that authenticated caller, **not an independent host observation of the tunnel**.
The host additionally holds the parent/service process handles, reads SCM
PID/config and rechecks the listener owner every 250 ms and on upstream connect.

stdout contains only these fixed, bounded lines:

```json
{"type":"ready","schemaVersion":1}
{"type":"error","code":"start_refused"}
{"type":"error","code":"configuration_refused"}
```

`ready` follows successful native initialization/configuration/readback and an
active lease. It is not a traffic-test, anonymity or full-system isolation
claim. URLs, paths, UA/Client Hints and credentials are not emitted. The GUI
must handle error, EOF, missing ready and timeout as failure.

## Accepted profile subset

- `engineId`: `webview2`; no Edge/Chrome/CEF binaries or alternate channels.
- `identity`: `native`, `generated`, `manual`, or legacy `observed-exact`.
  Generated mode accepts no frontend UA/CH and derives the UA from the selected
  environment and the actual Chromium major, using the reduced `major.0.0.0`
  UA version. Windows/macOS/Android use fixed reduced platform tokens; without
  an explicit environment the generated UA uses the reduced Windows template.
  Detailed selected platform/model and actual full versions remain in CH.
  Manual mode accepts `userAgent` as nonblank printable ASCII, at most 1024
  bytes, preserves it exactly, and rejects frontend Client Hints. It does not
  silently substitute the native UA. Settings.UserAgent and CDP apply the UA;
  navigator and an app-owned HTTP request must read back the same text before
  the relay opens. All subsequent HTTP requests are checked too. CH always
  comes from the real runtime plus selected environment, not from manual text.
  Native always retains the independently observed raw UA, even with a selected
  environment. That environment still independently changes CH/platform/
  viewport/touch, and can differ from the retained UA; the preview warns about
  this. Legacy observed-exact snapshots keep exact-match validation. No mode
  changes the real engine or hardware.
- Optional `environment`: absent or `{ "mode": "native" }` preserves native
  identity. `mode: "browser-emulation"` accepts the four finite OS/device
  pairs below and always uses an independently observed native runtime as its
  CH/version source. Native, generated and manual identity may use emulation;
  observed-exact cannot. No frontend engine version or Client Hints is accepted.
- `region`: one of the existing six complete presets. Environment language,
  CDP Accept-Language, locale and timezone are applied, then JS-visible values
  are read back before navigation is enabled. Failures close the session.
- `viewport`: native content dimensions without environment emulation. With
  emulation it is the actual CDP CSS viewport/screen size. Desktop range is
  800..3840 by 600..2160, mobile 320..3840 by 568..2160. Native DPI, physical
  placement, CDP device metrics and per-target inheritance require VM acceptance.
- `webrtc`: `disable-non-proxied-udp` only. This is the Chromium IP handling
  policy, not a claim that all WebRTC APIs are removed.
- `webgl`, `cookies`: `runtime-default` only.
- `webgpu`: `runtime-default` or `disabled`. The latter requests the native
  Chromium `--disable-features=WebGPUService` feature gate and requires actual
  app-owned secure-document readback before ready. `navigator.gpu` may remain
  visible; default, forced-fallback, high-power and low-power `requestAdapter`
  calls must all resolve to `null`, or the API must be absent. An available
  adapter, rejection, missing readback or timeout refuses the session. No JS
  deletion/stub of the WebGPU API is used to create that result. Runtime
  compatibility and native traffic/adapter behavior remain VM acceptance items.
- Optional `graphics`: absent or `{ "mode": "native" }` leaves WebGL names
  native. `mode: "webgl-metadata"` requires `webgpu: "disabled"`, a `presetId`,
  `vendor`, and `renderer`. `presetId: "manual"` accepts nonempty printable
  ASCII strings, with no leading/trailing whitespace, vendor at most 128
  characters and renderer at most 256. The three named presets below require
  both strings to match exactly; extra graphics fields are rejected.
- All five requested permissions: `block`. Additionally every WebView2
  PermissionRequested event is denied, including future/unlisted kinds.
- `binding`: loopback-proxy with exact `socks5://127.0.0.1:<proxyPort>` and no
  direct fallback. The ordinary branch binds the installed SCM service. A
  `referenceId: "profile-relay"` branch binds the authenticated GUI's local
  relay as described below; the actual HTTP/HTTPS/SOCKS upstream and its
  credentials remain in Go. The host never accepts a remote proxy URL.
- `exitCleanup`: retain-profile only. A safe random fresh private UDF is
  created for each launch under current-user LocalAppData; no existing profile
  is reused, read, migrated or deleted. InPrivate does not mean no disk traces.
  Directory LIST_DIRECTORY handles, denying DELETE sharing, pin the path from
  the volume root through the UDF for the session. Each component is opened
  relative to its already pinned parent with `NtCreateFile`; type, owner and
  DACL are checked through that returned handle. The two fixed containers use
  `FILE_OPEN_IF` and may already exist if their handle checks succeed. The
  actual random UDF leaf uses `FILE_CREATE | FILE_DIRECTORY_FILE`: creation and
  acquisition of its no-DELETE-sharing handle occur in one operation, and the
  result must explicitly be `FILE_CREATED`. A collision/existing-object result
  is refused, without reopening or retrying it. There is no create-then-path-pin
  gap in which an old ordinary profile directory can replace the new leaf.
  Runtime compatibility and actual sharing behavior of these native operations
  remain VM items. This does not defend against an attacker who
  already controls the same user token and can modify runtime-owned contents.
- A validated randomizationSeed is retained only as metadata explaining the
  caller's finite preset choice. It enables no additional runtime overrides.

The Go caller's pre-launch code verification must include the `.exe.config`
along with the EXE and three SDK/loader DLLs. CLR configuration can be processed
before `Main`; the host's own later ACL check cannot replace that caller check.

## WebGL metadata scope and readback

The supported named presets are:

| presetId | vendor | renderer |
| --- | --- | --- |
| `intel-uhd-620-d3d11` | `Google Inc. (Intel)` | `ANGLE (Intel, Intel(R) UHD Graphics 620 Direct3D11 vs_5_0 ps_5_0, D3D11)` |
| `nvidia-rtx-3060-d3d11` | `Google Inc. (NVIDIA)` | `ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11 vs_5_0 ps_5_0, D3D11)` |
| `amd-rx-6600-d3d11` | `Google Inc. (AMD)` | `ANGLE (AMD, AMD Radeon RX 6600 Direct3D11 vs_5_0 ps_5_0, D3D11)` |

For example, the relevant profile fields can be:

```json
{"webgl":"runtime-default","webgpu":"disabled","graphics":{"mode":"webgl-metadata","presetId":"manual","vendor":"Example vendor","renderer":"Example renderer"}}
```

A fixed, JSON-data-encoded script is registered and awaited through
`AddScriptToExecuteOnDocumentCreatedAsync` before bootstrap navigation. It
wraps the WebGL1/WebGL2 prototype `getParameter` methods and changes only
successful string results for `UNMASKED_VENDOR_WEBGL` (`0x9245`) and
`UNMASKED_RENDERER_WEBGL` (`0x9246`). Native receiver checks, numeric argument
conversion, missing/unknown parameters, unavailable extensions and the
original property attributes are preserved. No caller-provided script runs.

The startup probe reads actual WebGL1/2 contexts in the main window, a fixed
same-origin URI frame, `srcdoc`, and `about:blank`, including window
`OffscreenCanvas` paths when available. Both main-window regular contexts and
their debug-renderer extension must be available with exact requested values;
otherwise metadata mode is refused. For the other paths, an unavailable
context is reported separately and never counted as a match. A supported
constructor with a missing wrapper is refused even if its context cannot be
created. The same four secure documents check native WebGPU adapter blocking.
The host uses its own intercepted HTML only, with no website host bindings.
The probe frames are removed before ordinary navigation is enabled.

The registration API covers future frame documents, but its return alone is
not evidence that every runtime frame/worker path is covered. The above probes
are startup checks; real cross-origin OOPIF creation, document-start timing,
worker/service-worker target closure, and subsequent navigation still require
the controlled VM fixtures. The host has not been run locally and those
fixtures have not passed here. Pure mocks cannot supply that evidence.

This is a detectable JavaScript metadata override. It does not change the
physical adapter, driver, acceleration, supported capabilities, actual rendered
pixels or performance. It provides no GPU fingerprint anonymity guarantee and
does not turn WebView2 into a customized Chromium engine. The Chinese window
summary displays this scope and the actual successful/unavailable probe counts.

## Runtime gates and limits

The runtime path is discovered only from the machine stable WebView2
registration, must match the standard Program Files EdgeWebView/version path,
and must pass trusted ACL/owner/no-reparse checks. Per-user registrations,
unknown locations, beta/dev/canary and WebView2 environment/policy overrides
are refused. The bundled official loader is selected explicitly.

The engine's only configured proxy is a new session-owned IPv4 loopback relay.
It starts closed. Each frontend connection must first have a real client-side
TCP owner tuple whose PID appears in this fresh UDF's official WebView2
`GetProcessInfos` list as browser, utility or renderer. Its executable must
match this fixed runtime, be alive and have an ancestry chain to the pinned
`CoreWebView2.BrowserProcessId`; creation-time ordering rejects a parent PID
recycled after its child. Each process handle in that chain remains held until
the connection closes, and the client tuple is read again after pinning. An
unknown local process is closed without sending any upstream credentials;
the public loopback port is never treated as a bearer secret.

The ordinary branch forwards only to the fixed loopback service port. The
custom profile branch instead requires these extra first-frame fields:

```json
{"relayPid":4321,"relayUsername":"ultra-browser","relayPassword":"<64 lowercase hexadecimal characters>"}
```

`relayPid` must equal the actual stdin-pipe-authenticated installed GUI parent;
it is not a caller-chosen arbitrary PID. Its server-side connection/listener
tuple must match that still-open parent handle. Credentials are accepted only
with `profile.binding.referenceId: "profile-relay"`, all three fields are
required, and ordinary service bindings must omit them. Both branches still
require the real installed SCM service and ongoing connected/strict Ultra
lease. **Custom proxy is currently layered over Ultra, not a VPN-independent
mode.** The host does not receive the actual custom upstream host/user/password.

Chromium SOCKS username/password support is not assumed. The C# relay consumes
the authorized WebView's no-auth greeting, independently negotiates the Go
relay's RFC1929 username/password authentication, and sends WebView a no-auth
accept only after successful authentication and renewed owner checks. It then
forwards CONNECT and remaining bytes. The SCM branch separately negotiates its
existing no-auth method. Authentication refusal, method downgrade, malformed or
truncated frames close the connection/session; there is no unverified passthrough.

Each upstream connection must match the trusted PID's exact server-side
local/peer tuple before browser bytes are forwarded. PID reuse cannot resurrect
a closed process handle. There is no direct or system
proxy path in this relay. Chromium flags remove implicit loopback bypass,
block native hostname resolution and QUIC, and restrict non-proxied WebRTC UDP.
They do not constitute a kernel-wide firewall or proof that every future
runtime component honors proxy flags; full browser traffic and failure tests
are mandatory before making a network-protection claim.

The initial secure bootstrap page is an in-memory response at a reserved
`.invalid` origin, intercepted while all network requests/relay are closed.
It has no external resources, site script or host binding. It allows native
Client Hints observation without visiting a website. Request interception,
sandbox and certificate behavior still need actual runtime acceptance.

New windows, external URI protocols, downloads, HTTP authentication, client
certificate selection and TLS-error bypass are refused. Host objects, web
messaging, DevTools, browser accelerators, password saving and autofill are
disabled. SmartScreen/reputation checking remains required; sandboxing and
certificate checks are never disabled.

CDP auto-attach pauses new targets; each attached event must explicitly report
`waitingForDebugger=true`, otherwise the session closes. Iframe targets must receive and read back
the same parameters before release. Workers/service workers/new page targets
are closed without resuming; unsupported/CDP failure closes the host. This
intentionally limits sites. OOPIF/event/session coverage, absence of execution
before configuration, closure of all worker variants and target teardown are
unverified VM items. The host must not be offered as accepted/production-safe
until those tests and Go lifecycle integration pass.

Official API references used for this implementation:
[environment options](https://learn.microsoft.com/en-us/dotnet/api/microsoft.web.webview2.core.corewebview2environmentoptions),
[settings](https://learn.microsoft.com/en-us/dotnet/api/microsoft.web.webview2.core.corewebview2settings),
[document-created script registration](https://learn.microsoft.com/en-us/dotnet/api/microsoft.web.webview2.core.corewebview2.addscripttoexecuteondocumentcreatedasync),
[WebGL debug-renderer constants and Window/Worker exposure](https://registry.khronos.org/webgl/extensions/WEBGL_debug_renderer_info/),
[Chromium WebGPUService feature](https://chromium.googlesource.com/chromium/src/+/refs/heads/main/gpu/config/gpu_finch_features.cc),
[Chromium native WebGPU adapter path](https://chromium.googlesource.com/chromium/src/+/refs/heads/main/third_party/blink/renderer/modules/webgpu/gpu.cc),
[user data folders](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/user-data-folder),
[Chromium SOCKS/DNS scope](https://www.chromium.org/developers/design-documents/network-stack/socks-proxy/),
[Chromium proxy bypass rules](https://chromium.googlesource.com/chromium/src/+/HEAD/net/docs/proxy.md),
[NtCreateFile relative creation/returned handle contract](https://learn.microsoft.com/en-us/windows/win32/api/winternl/nf-winternl-ntcreatefile).

The local pure suite currently reports **624 C# checks** and **36 JavaScript
mock checks**. The latter exercise JSON data safety, native method receiver/
argument/error behavior, WebGL1/2 and OffscreenCanvas probes, and rejection when
mock native WebGPU still returns an adapter. They do not prove real runtime
behavior. Native-directory checks
use an injected fake `NtCreateFile` delegate and synthetic security descriptors:
x64 structure offsets, parent handle, UTF-16 component, security descriptor at
creation, no-delete share flags, fresh/open-if dispositions, returned-created
status, collision/pending/error refusal, no path reopen/retry, invalid component
rejection, and owner/DACL boundaries. This exercises marshalled requests and
failure handling; it does not execute `NtCreateFile`, create a real directory,
or prove OS/runtime sharing compatibility.

## Four browser-visible OS environments

These are controlled web-environment simulations in Windows Chromium/WebView2.
They do not install or run macOS, Android or iOS, do not implement native Safari
or WebKit, and do not promise identical fonts, rendering pixels, sensors,
codecs, Canvas/Audio or an undetectable physical device fingerprint.

| os / devicePresetId | default osVersion | navigator.platform / UA-CH platform | architecture / model | DPR / touch | recommended CSS viewport |
| --- | --- | --- | --- | --- | --- |
| windows / windows-desktop | 10.0.0 | Win32 / Windows | x86 / empty | 1 / 0 | 1280×720 |
| macos / macos-desktop | 13.0.0 | MacIntel / macOS | x86 / empty | 1 / 0 | 1280×720 |
| android / pixel-7 | 13.0.0 | Linux armv8l / Android | arm / Pixel 7 | 2.625 / 5 | 412×915 |
| ios / iphone-14 | 16.0.0 | iPhone / iOS | arm / iPhone | 3 / 5 | 390×844 |

For example `environment` can be
`{"mode":"browser-emulation","os":"ios","devicePresetId":"iphone-14","osVersion":"17.2"}`.
The optional OS version accepts 1–3 decimal segments, each 0..999 without
leading zeroes, with a positive major, then normalizes to three segments.
Unknown pairs/fields, supplied runtime versions and observed-exact identity
combined with emulation are refused.

The host first observes native UA, navigator.platform and high-entropy CH
including `model` in its app-owned HTTPS bootstrap. It extracts the real
Chromium full version from the observed `fullVersionList` Chromium brand,
checking its major against both actual Chrome UA and WebView2 runtime major.
The full Edge runtime version may differ from the Chromium full version and
is never substituted for it. Generated mode produces `Chrome/<actual major>.0.0.0`:
Windows freezes the UA token to NT 10.0, macOS to 10_15_7 and Android to Android
10 / K. The detailed selected OS/model remains in CH. The iOS option keeps a
synthetic iPhone OS token plus the actual reduced Chrome major; it does not
claim retail CriOS, native Safari or WebKit 605. Native and manual UA text remain
verbatim even with environment emulation. Observed native Chromium
and GREASE brands/versions are retained; the Edge product brand is removed for
the Chrome-form UA. No fabricated Safari Version/ or WebKit engine version is
introduced; the ordinary Blink compatibility tokens remain. iOS/Android CH on
this Windows runtime is detectable simulation.

The Windows `osVersion` field keeps its existing raw UA-CH platform API-contract
meaning. It is not silently translated from the marketing name Windows 11 into
13.0.0. UI presets may explicitly select Windows 10 (10.0.0), Windows 11
(13.0.0), or a custom platform version. The UA stays NT 10.0 in generated mode.
See the official [Chromium UA reduction reference](https://www.chromium.org/updates/ua-reduction/),
[current Chromium token builder](https://github.com/chromium/chromium/blob/main/components/embedder_support/user_agent_utils.cc)
and [Microsoft Windows platform-version explanation](https://learn.microsoft.com/en-us/microsoft-edge/web-platform/how-to-detect-win11).
Go's `GeneratedChromiumUserAgent` is shared by the native Chrome host. Its nine
golden vectors in `internal/browserhost/testdata/chromium-ua-golden.json` are also
embedded in the C# pure test assembly; the two implementations must match the
same complete expected strings. Unknown-runtime Go previews use only the fixed
`{ChromiumMajor}.0.0.0` placeholder and remain unapplied plans.

Actual settings/CDP calls apply UA, `Network.setUserAgentOverride` metadata
(including required `model`), platform, locale/timezone, device metrics and
touch emulation. Before ready, app-owned JS reads back UA, CH, navigator
platform, language list, locale/timezone, CSS viewport, screen dimensions, DPR
and touch points. An unsupported call or mismatch refuses navigation. Iframes
are configured and read back before their paused target is resumed; the
top-level device metrics must also be inherited correctly by each OOPIF.
All worker/service-worker/new-page targets remain paused and are terminated,
never resumed with an unverified environment. Sites requiring them may fail.

Mobile sessions use a real narrow WinForms ClientSize and two-row address/
navigation controls. The physical content window scales to available work
area while the CDP CSS viewport, screen size and DPR retain the preset/user
values; too-small displays refuse startup. This is actual host layout and
CDP output scaling code, not a preview-only setting. No local browser was
opened to verify it; VM checks must confirm physical placement, clipping,
input mapping, mobile viewport and touch on real pages, new documents and
cross-origin frames before UI can claim acceptance.

The added pure checks cover four OS contracts, real-versus-Edge Chromium
version derivation, model/platform/mobile alignment, finite OS versions,
actual metrics payload and layout fit math, reversed frontend TCP rows, strict
relay credentials, RFC1929 frames and refusal of authentication downgrade.
Their in-memory duplex streams do not open a socket or execute a native API.
The new real-process/pipe/TCP-owner checks remain unexecuted VM items.

Additional primary references:
[WebView2 process inventory](https://learn.microsoft.com/en-us/dotnet/api/microsoft.web.webview2.core.corewebview2environment.getprocessinfos),
[CDP device metrics and touch](https://chromedevtools.github.io/devtools-protocol/tot/Emulation/),
[CDP UA/platform/Client Hints](https://chromedevtools.github.io/devtools-protocol/tot/Network/#method-setUserAgentOverride),
[SOCKS5 username/password subnegotiation](https://www.rfc-editor.org/rfc/rfc1929).
# CPU count emulation

The optional `profile.hardware` object is exactly
`{"mode":"logical-processors","logicalProcessors":8}`, with an integer from
1 through 64. Omit the field for native behavior. Null, aliases, duplicate or
extra fields, fractions/exponents, other modes and out-of-range values are
refused. This is a website-visible logical processor count, not a replacement
CPU or performance change. No RAM override is provided.

`HardwareOptions.ConfigureTarget` executes the official experimental
`Emulation.setHardwareConcurrencyOverride` command, then independently reads
`navigator.hardwareConcurrency` through `Runtime.evaluate`. It runs before the
owned initial document and again for the main target and each paused iframe
target. A missing command, protocol error, inaccessible realm, timeout or
readback mismatch prevents the navigation gate from opening (or revokes the
existing session). Omission executes no new CPU protocol command.

While the relay is closed, the owned in-memory identity page creates same-origin,
srcdoc and about:blank frames. A read-only probe checks all three plus the main
frame against the requested count. These frames are removed before related
target auto-attach is enabled. Later OOPIF targets must already be paused, receive
the command and pass their own readback before resume; worker targets remain
blocked before execution. No navigator getter or prototype is patched for CPU.

Primary protocol evidence is the official
[CDP Emulation PDL](https://github.com/ChromeDevTools/devtools-protocol/blob/master/pdl/domains/Emulation.pdl)
(`setHardwareConcurrencyOverride`, integer `hardwareConcurrency`, experimental)
and the official [Target auto-attach PDL](https://github.com/ChromeDevTools/devtools-protocol/blob/master/pdl/domains/Target.pdl).
An installed runtime's support and renderer/OOPIF behavior must still be tested
in the isolated VM. Static compilation and mocked protocol/Node tests do not
prove that runtime behavior. The pure test executable does not link or execute
WebView2; its CPU tests use injected replies and frozen mock navigator objects.

