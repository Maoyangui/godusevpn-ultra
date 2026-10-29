# QA12 source: native Gecko WebGL metadata and WebGPU blocking

This is a source checkpoint, separate from immutable QA11. No browser, product
binary, Job, firewall, service, installation or VM was executed for this work.
Pure protocol/DOM/Node mocks establish code behavior, not real Firefox or GPU
acceptance. The ordinary native launch gate remains unchanged.

The four user reference images were read in full. Their hardware panel asks for
WebGL vendor/renderer, WebGPU, CPU, RAM, device name and MAC. The other panel
lists Canvas, WebGL images, AudioContext, media devices, ClientRects and voices.
This batch implements the Gecko name and WebGPU controls; it does not silently
add RAM, synthetic devices, six noise switches, or a custom Chromium engine.

## Implemented contract

Native `Config` and the fixed GUI bridge accept optional `graphics` with the
existing `mode`, `presetId`, `vendor`, `renderer` structure, and optional
`webgpu: "runtime-default" | "disabled"`. Omission retains native behavior.
Only Firefox accepts metadata/disabled requests in this native host. Chrome
refuses those requests before launching. The UI announces this before editing,
disables unsupported choices, preserves an existing draft across engine changes,
and allows explicitly returning to native. Main and legacy launch paths use
the same complete request.

Metadata is still a paired printable-ASCII display-string contract: vendor
1..128 and renderer 1..256, no leading/trailing whitespace. A named preset must
match both fixed strings exactly. Named Windows D3D templates remain refused for
non-Windows emulated environments. Unknown, duplicate/case-alias, null or extra
bridge fields are refused. These templates are names, not physical GPU or
retail Firefox/Chrome identities.

Only an explicit metadata request writes these preferences into the newly owned
Firefox profile before browser creation:

```
webgl.override-unmasked-vendor = <the validated vendor>
webgl.override-unmasked-renderer = <the validated renderer>
webgl.sanitize-unmasked-renderer = false
```

`webgpu: "disabled"` separately writes `dom.webgpu.enabled=false`. No navigator
or WebGL method is replaced by this backend. Native mode adds none of these
preferences. No RFP/FPP or privileged-extension preference is changed.

## Actual upstream implementation and limits

Read official Firefox `FIREFOX_147_0_1_RELEASE`, `FIREFOX_158_0_1_RELEASE`, and
current commit `f4970b3baab87e71af27f3f919abe49c86cfb83c` directly. All contain
the two override preferences and real native getter branches. In Firefox147
[ClientWebGLContext.cpp](https://github.com/mozilla-firefox/firefox/blob/FIREFOX_147_0_1_RELEASE/dom/canvas/ClientWebGLContext.cpp#L2377)
the two native helpers read the preferences, and the extension's unmasked
0x9245/0x9246 cases use them. Renderer sanitization is applied afterwards unless
explicitly disabled. Defaults and types are in
[StaticPrefList.yaml](https://github.com/mozilla-firefox/firefox/blob/FIREFOX_147_0_1_RELEASE/modules/libpref/init/StaticPrefList.yaml#L18916).

Masked `VENDOR` remains Gecko's `Mozilla`; ordinary `RENDERER` follows Gecko's
own renderer-query/sanitization rule and may also reflect the supplied renderer
through that rule. The two masked fields are not promised to equal the requested
unmasked pair. RFP may hide the debug extension or substitute `Mozilla`;
[Firefox158 adds vendor sanitize/constant/randomization and renderer-constant branches](https://github.com/mozilla-firefox/firefox/blob/FIREFOX_158_0_1_RELEASE/dom/canvas/ClientWebGLContext.cpp#L2500).
Those branches are not silently disabled: a mismatching readback refuses this
custom request. Source presence in a version is not a runtime qualification.

WebGPU's
[Window/Worker WebIDL](https://github.com/mozilla-firefox/firefox/blob/FIREFOX_147_0_1_RELEASE/dom/webidl/WebGPU.webidl#L87)
uses `Instance::PrefEnabled`; the
[native implementation](https://github.com/mozilla-firefox/firefox/blob/FIREFOX_147_0_1_RELEASE/dom/webgpu/Instance.cpp#L39)
checks the same preference and also rejects adapter requests when disabled.
The new Window readback requires native interface absence; it does not fabricate
absence with JavaScript or accept an arbitrary exception as successful blocking.

## Readback before opening the relay

Existing native-version, proxy, two actual HTTP-UA memory responses, locale,
timezone, UA and environment checks still run. A graphics request additionally
reads real getters in four app-owned Window scopes: the secure main page,
same-origin srcdoc, opaque data frame, and a second secure HTTPS origin. The last
uses a fresh unpredictable URL under `godusevpn-graphics.invalid` and only that
exact request is fulfilled in memory by BiDi. Its actual context, destination
and HTTP UA are verified; the response must succeed independently of the probe's
JSON. The relay stays closed and no website/DNS request is authorized by this
fixture mechanism. Unknown client-supplied origins/scripts/URLs are not accepted.

Each metadata scope checks WebGL1/2 on ordinary Canvas and Window OffscreenCanvas.
Normal contexts and the debug extension must be usable and both strings must
match. An existing API returning null, throwing, or losing the extension refuses
metadata. A genuinely missing OffscreenCanvas API is explicitly counted as
unavailable, never as a string match. Any used context is released. WebGPU
absence is checked in secure main/same/cross scopes; the insecure opaque frame
cannot independently prove a secure-only API was disabled.

Worker/SharedWorker construction, worklets, service-worker registration and
non-Window realms retain the existing denial. Each Window probe independently
checks Worker construction is blocked. No worker is temporarily enabled to test
GPU access. Evidence explicitly reports `workerApiRuntimeReadback:false`:
Worker coverage is execution blocking plus the upstream shared native gate,
not a claimed Worker API runtime measurement. There is no claim that BiDi
realm-created events pause arbitrary page execution or that every future site
realm has a new pre-execution runtime measurement. Native preferences apply at
engine level; the four startup fixtures exercise known scope classes.

Successful status carries only implementation, scope/context counts and these
boundary flags, not a newly collected hardware fingerprint. A failed graph
returns the fixed `graphics_readback_failed` code. The existing serialized
startup teardown retains ownership unless Job/filter cleanup is confirmed.

## Verification and remaining acceptance

Pure tests cover strict names/capability/bridge fields, no changes on omission,
JSON-safe native preferences, no RFP/FPP weakening, each scope mismatch, null or
missing contexts/extensions, WebGPU and Worker evidence, cross-origin response
failure/wrong UA/missing independent fulfillment, context/API availability
distinction, defensive copies, and fixed-code partial-owner cleanup.

The production probe source runs in a Node `vm` mock with four scopes and
ordinary/Offscreen WebGL1/2. It verifies unchanged getter descriptors and context
release, without executing Firefox. DOM fixtures cover complete Firefox
metadata/WebGPU-only forwarding, both launch entry points, early Chrome
capability notice, preserved drafts and existing UA/environment behavior.

VM acceptance must still run Firefox147/158 native and metadata requests, secure
same/cross/opaque frames, Offscreen paths, WebGPU-only blocking, RFP/FPP conflict,
missing-GPU/context refusal, later navigation, proxy/lease loss and cancel/close
cleanup. No new public launch qualification, package or VM result is claimed.

## Other reference-image gaps

| Surface | Current source / next action |
| --- | --- |
| CPU | WebView2/Chrome native protocol override; Firefox custom still refused. Gecko `dom.maxHardwareConcurrency` is a cap, not arbitrary exact core-count emulation. |
| RAM | No editable backend. `navigator.deviceMemory` is an estimate, not installed RAM. Current Chromium [native calculation](https://github.com/chromium/chromium/blob/main/third_party/blink/common/device_memory/approximated_device_memory.cc) uses platform-dependent coarse bounds; the reviewed [CDP Emulation](https://chromedevtools.github.io/devtools-protocol/tot/Emulation/) has no memory override. A future implementation must reconcile Window/Worker and memory Client Hint headers; a JS getter alone must be labelled limited metadata. |
| Device name / MAC | No corresponding browser configuration or real backend. A profile name, UA-CH device model and Windows hostname are different surfaces; adding invented navigator properties would not change the system NIC/hostname. Define the actual exposure and readback before adding a control. |
| Six noise switches / fonts | No implemented per-surface backend. [Firefox's native fingerprinting protection](https://support.mozilla.org/en-US/kb/firefox-protection-against-fingerprinting) supplies specific Canvas/font defenses, but can also change CPU/touch/identity; it needs a separately qualified mode and conflict handling. It is not six arbitrary noise controls. [Brave's engine implementation](https://brave.com/privacy-updates/38-webgl-webgpu-fingerprinting-protections/) is another auditable native route, with a different engine/build/update scope. |
| WebGPU “based on WebGL” | Not implemented; a renamed WebGL string does not select a compatible real WebGPU adapter, capabilities or pixels. Current supported alternative is native WebGPU blocking. |
| WebView cross-origin graphics | Existing C# graph fixture covers owned main/same/srcdoc/aboutblank. Its later OOPIF configuration reads identity/CPU/environment, not a separate graph probe. Do not count this as completed cross-origin GPU runtime acceptance. |

OS presets remain webpage environment emulation on the real Windows engine.
No Safari, retail iOS kernel, changed physical GPU/driver, rendering-output or
performance anonymity is provided by these metadata controls.
