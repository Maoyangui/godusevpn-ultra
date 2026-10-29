# Native browser host — controlled native launch path

Windows amd64/arm64 builds expose `Start`, which uses the installed ordinary
GUI's existing launch API and the same real `StartAcceptance` path. It skips no
GUI/token/helper/code/Job/WFP/proxy/protocol check and is not a frontend bypass
parameter. The acceptance build tag remains as a compatibility marker for QA
packages; it does not bypass or weaken runtime checks. Real browser, proxy and
traffic behavior still needs the controlled Windows VM matrix before it can be
described as generally verified.

Each session has a random instance path and a new private profile. Code comes
from a fresh official archive's in-memory manifest, rather than trusting the
ordinary user's cached receipt. The GUI holds exact tree pins; the fixed
administrator helper independently pins every object, sets Administrators
owner and a protected exact DACL (SYSTEM/admin full; GUI user read/execute),
then re-enumerates and reads owner/DACL/final path on the same handles. Profile
remains a separate writable leaf. This prevents ordinary additions of optional
DLLs to the sealed code tree. It does not claim protection from a compromised
complete current-user token or an administrator. A reusable protected machine
template is still unfinished: acceptance downloads the official archive again
for each instance. This must not be described as the finished cache UX.

CreateProcess uses startup JOB_LIST and CREATE_SUSPENDED, with kill-on-close and
no breakaway. The authenticated helper derives actual user paths from its held
GUI token, including alternate-administrator UAC; before Resume it seals code
and installs independently read-back persistent complement BLOCK rules for
every instance EXE. Its independent WFP namespace leaves only IPv4 loopback TCP
to this session's SOCKS frontend. Firefox also requires a loopback BiDi listener.

The initially closed no-auth SOCKS frontend uses reverse TCP tuple, live PID,
Job membership, creation time, exact code image and user SID to authorize the
real browser client. It checks the backend belongs to this GUI process before
and after random RFC1929 authentication. Credentials never enter browser args
or profile files. CONNECT/domain bytes pass to the explicit upstream proxy;
the frontend does no website DNS or direct destination dial. It opens only
after controlled HTTP/navigator probes and the second WFP readback succeed.

Chrome uses actual private ASCIIZ CDP pipes. Firefox uses actual text WebDriver
BiDi `/session`, after validating its new profile's endpoint file and the real
loopback server PID/Job/image. Mozilla's control server accepts no web Origin
by default and provides no authentication; this is a trusted-local-account
boundary, never a public remote automation endpoint. No caller can supply an
executable, path, arbitrary CLI switch, script, PAC or control URL.

Identity uses `identity:{mode:native|generated|manual,userAgent?}` and the fixed
`environment:{mode:native|browser-emulation,os,devicePresetId,osVersion}` data.
Caller-supplied Client Hints are refused. Chrome's actual high-entropy UAData
supplies engine brands/versions; formal CDP applies UA/platform/UA-CH, metrics
and touch. Firefox uses formal BiDi UA/locale/timezone/viewport plus native
fresh-profile preferences for platform/OSCPU/touch/meta viewport. A version
that ignores a requested setting or lacks a required command refuses startup.
Manual UA remains exact printable text; header normalization/mismatched
navigator must fail. Firefox never acquires fabricated Chromium UAData.
Random owned HTTPS documents are fulfilled in memory to verify the actual HTTP
UA; main/same-origin/opaque-frame native values are checked before readiness.
UAData availability follows secure-context rules, without fabricating an API
for opaque non-secure documents.

Only `generated` creates a UA. `native` preserves the independently read UA
verbatim, including when an environment separately overrides platform/CH/
viewport/touch; the preview warns that those fields can then describe different
environments. Modern generated Chrome uses the same Go builder as WebView2:
`Chrome/<major>.0.0.0`, frozen NT10.0 / macOS10_15_7 / Android10;K UA tokens.
The exact live product and Client Hints full versions stay intact. Windows
OSVersion retains its UA-CH contract semantics and never becomes an NT version.
Firefox uses modern NT10.0 and Macintosh/10.15 desktop tokens with Gecko20100101;
Android includes its selected release (below10 reports10), Mobile, and GeckoM.0.
The fixed >=158 policy strips Android minor components; older versions trim
normalization padding but retain nonzero release components. iOS keeps its
explicit synthetic environment with the actual Chrome or Gecko/Firefox product
token. It is not a retail CriOS/FxiOS/Safari identity and is labelled accordingly.
These policies follow [Chromium UA reduction](https://www.chromium.org/updates/ua-reduction/)
and fixed [Firefox147](https://raw.githubusercontent.com/mozilla-firefox/firefox/FIREFOX_147_0_1_RELEASE/netwerk/protocol/http/nsHttpHandler.cpp)
/ [Firefox158](https://raw.githubusercontent.com/mozilla-firefox/firefox/FIREFOX_158_0_1_RELEASE/netwerk/protocol/http/nsHttpHandler.cpp)
source. UA policy tests and memory-protocol HTTP/frame readback tests establish
local code behavior; real browser behavior still requires the VM matrix.

Optional `hardware:{mode:"logical-processors",logicalProcessors:1..64}` requests
Chrome's webpage-visible `navigator.hardwareConcurrency`; omission keeps the
native value. Firefox refuses this custom request before protocol mutation.
Chrome uses the official experimental
[setHardwareConcurrencyOverride](https://chromedevtools.github.io/devtools-protocol/tot/Emulation/#method-setHardwareConcurrencyOverride)
command, then reads the native property in every paused main/OOPIF target before
resuming it. A missing execution context, unsupported command or mismatching
value revokes the session. The existing two owned HTTP UA bootstrap checks still
run, and main, same-origin srcdoc and opaque data frames must each return the
requested integer before readiness. Workers stay blocked. No JavaScript getter
replaces the hardware property. This changes neither physical cores nor browser
processing capacity/performance. Pure transport tests cover command/read ordering,
per-frame mismatches and later OOPIF refusal; actual paused-target and multi-frame
behavior still requires VM acceptance and does not change the public launch gate.

Four OS presets emulate selected webpage-visible metadata/viewport/touch. They
do not change the real OS, physical GPU, engine, pixels or performance, and do
not supply Safari/WebKit. QA12 source now implements Firefox metadata using its
native `webgl.override-unmasked-vendor/renderer` preferences and separately
disables native WebGPU with `dom.webgpu.enabled=false`. Only explicit metadata
sets `webgl.sanitize-unmasked-renderer=false`; RFP/FPP stays intact and any
conflicting result refuses startup. Stock Chrome graphic requests remain
explicitly unsupported in this host. Four owned Window scope classes and
Canvas/Offscreen WebGL1/2 are read before opening the relay; no JS getter is
replaced. Worker execution stays blocked, and evidence explicitly says no Worker
API runtime readback. Masked values, actual rendering and hardware remain under
Gecko's own rules. These source changes require new VM acceptance and do not
change immutable QA11. See the [exact scope and evidence](../../docs/ultra/qa/FIREFOX_NATIVE_GRAPHICS_QA12_2026-10-27.md).
The driver constrains permissions,
downloads, workers/service-worker registration, extra tabs and non-HTTP(S)
navigation. Its proxy contract covers browser web traffic, not an external OS
application explicitly opened outside this browser. Real redirects, DNS,
WebRTC, worker/realm and user-interaction behavior remain VM acceptance items.

Failure closes both relays, terminates the Job and confirms ActiveProcesses
zero before closing synchronous pipes, asking the helper to remove intact owned
filters/check absence, then releasing pins. Unknown Job state retains the whole
ownership graph against GC; read-only observation may finish after actual zero.
Unknown removal is never retried or called success. Helper peer-loss actively
terminates its duplicated Job. The WFP provider preserves the established nil-
service design and checks exact flags/ownership; BFE/reboot needs VM evidence.

The GUI controller's `BrowserEngineProfileBeginLaunch` starts the same backend
as the blocking compatibility `BrowserEngineProfileLaunch`, while returning a
fresh random `startId` and `canCancelStart`. `BrowserEngineProfileCancelStart`
accepts only that current startup ID; an old callback cannot cancel a newer
attempt or close an already published ready session. Cancellation immediately
cancels the shared download/start context and returns without waiting for Job
shutdown. The startup goroutine alone owns partially assigned native resources;
it checks cancellation between resource acquisitions and releases them serially.
A Session returned late is closed once by the controller. Status remains readable
while startup or cleanup is pending. Cancelled attempts never reuse their random
native session or fresh-profile directories, and cleanup does not recursively
remove browser files.

`cancelling` is pending, and only `start_cancelled` means Start has returned and
any returned Session reports strictly `closed`, no readiness/navigation, and a
closed Done channel, with Close completed without error. A startup error returns its Session owner whenever cleanup
is unconfirmed, including `closed_filters_unknown` even though Done is closed.
These states keep the launch slot occupied; neither cancellation nor a second
Start retries unknown cleanup. `start_failed` retains one fixed stage code
(`host_not_ready`, `helper_required`, `version_store_unavailable`, or
`host_start_failed`) rather than exposing the original error, proxy address,
credentials or request. Invalid input is refused before a startup ID is issued.
Pure controller tests cover cancelled slow starts, stale IDs, late Sessions,
blocking/unknown cleanup and cancellation racing ready publication. Go's race
detector was unavailable in this local CGO-disabled environment; these checks
and repeated synthetic concurrency runs do not establish real VM teardown.

Local tests use pure memory protocol/SOCKS handshakes, synthetic Windows TCP
tables/security descriptors, fake lifecycle/filter ordering and inert TEMP
archives. No browser, real Job/WFP, ACL mutation, installer or system/network
change was run locally. These tests do not establish real traffic isolation or
a successfully launched browser window.

Primary contracts: [CDP Emulation](https://chromedevtools.github.io/devtools-protocol/tot/Emulation/),
[Firefox agent security](https://firefox-source-docs.mozilla.org/remote/Security.html),
[Firefox BiDi emulation](https://github.com/mozilla-firefox/firefox/blob/main/remote/webdriver-bidi/modules/root/emulation.sys.mjs),
[native navigator preferences](https://github.com/mozilla-firefox/firefox/blob/main/dom/base/Navigator.cpp),
[native max-touch-points](https://github.com/mozilla-firefox/firefox/blob/main/dom/ipc/BrowserParent.cpp),
[SetSecurityInfo](https://learn.microsoft.com/en-us/windows/win32/api/aclapi/nf-aclapi-setsecurityinfo).
