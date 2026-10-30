# Saved browser drafts

This package saves non-secret browser proposals for the current ordinary
Windows user. It does not launch a browser, query a VPN service, validate a
downloaded engine or restore a live network binding. A loaded record always has
`applied:false` and `requiresLaunchValidation:true`.

## Bridge contract

`BrowserProfilesList()` returns `{records:[],applied:false}`. Each summary has
`id`, `name`, `revision`, `engine`, `updatedAt` and `recoveryNeeded`.

`BrowserProfileSave(recordJSON)` accepts:

```json
{
  "schemaVersion": 1,
  "name": "工作环境 · 上海",
  "config": "a complete browserhost profile object, omitting binding",
  "engine": { "engineId": "webview2", "version": "" }
}
```

Schema version 2 keeps the same validated `config` draft and adds `meta` and
`extras` for the full browser-environment editor:

```json
{
  "schemaVersion": 2,
  "name": "社媒工作配置",
  "meta": {"browser":"sunbrowser","environmentName":"工作环境","group":"社媒","remark":"仅本地草稿","tags":["工作"]},
  "extras": {
    "storage": {"cookieFormat":"json","merge":true,"cookieCount":0,"cookieSessionOnly":true},
    "tabs": {"urls":["https://example.com"],"startupMode":"urls"},
    "proxy": {"mode":"no-proxy","type":"none","provider":"IP2Location","accountRef":""},
    "fingerprint": {"webrtc":"disable-non-proxied-udp","timezone":"real","geo":"real","language":"real","uiLanguage":"by-language","resolution":"ua","fonts":"default","noise":{},"webglMetadata":"real","webgpu":"real","seed":""},
    "advanced": {"cpu":"real","ram":"real","deviceName":"real","mac":"real","dnt":"default","portScan":"enabled","hardwareAcceleration":"default","tls":"default","teamApps":"global","dataSync":"global","browserSettings":"global","randomFingerprint":false,"allowedPorts":[]},
    "readback": {"state":"unknown","scope":"browser-profile","runtimeVerified":false,"warnings":[]}
  },
  "engine": {"engineId":"webview2","version":""}
}
```

上面为结构示意；实际请求仍必须携带完整的 `config` 对象，不能用字符串或省略运行时草稿。

`meta` and every `extras` section have exact keys, bounded lengths and enum
validation. Cookie values are intentionally not accepted in the persisted
schema: the UI parses JSON, Netscape and `Name=Value` text in memory and saves
only format, merge strategy and count. The editor displays unsupported or
unverified fields as such. Launch stops when a selected v2 field has no
reliable runtime adapter, so an unsupported setting cannot be silently
dropped. Schema 1 records remain loadable and are returned with empty v2
sections.

`config` above must be an object, not that explanatory string. New records omit
both `id` and `expectedRevision`. Updates and renames supply both, using the most
recent returned revision. The ID is a backend-generated `p-` plus 32 lowercase
hex digits. It is independent of the human display name. Names support Chinese
and other Unicode (1–80 runes), reject control/format characters and leading or
trailing whitespace, and are never used in a path. `config.profileId` is assigned
to that backend ID; at 34 characters it fits browserhost's 48-character limit.

`BrowserProfileLoad(id)` and successful saves return:

```text
{schemaVersion,id,name,revision,config,engine,updatedAt,
 applied:false,requiresLaunchValidation:true,recoveryNeeded:false}
```

`BrowserProfileDelete(id,expectedRevision)` returns `{id,deleted:true}`. A
revision conflict requires a reload or an explicit save as a new record; the
backend never silently overwrites a changed record. The UI should retain the
unsaved draft on every error and escape names as display text.

The independent engine selection is `webview2` with an empty version,
`chrome-for-testing` with a fixed four-component version, or `firefox` with a
fixed two/three-component version and optional `esr`. It is a selection to check
at launch, not a claim that this engine is installed or that all draft settings
are implemented by it. `config` remains the current `browserhost.ProfileConfig`
draft. Save/load uses that real pure validator, with a temporary in-memory
placeholder binding that is removed before persistence or return. Engine
availability is deliberately not required to save a draft.

Only `native`, `generated` and `manual` identity modes can be stored. Manual UA
text is preserved exactly after the existing printable-ASCII/length check.
`observed-exact` and all frontend Client Hints are rejected: an old runtime
observation must not become future launch trust. Binding (even null), service
PID, port, proxy configuration/credentials, subscription, paths, extra runtime
arguments, unknown fields, duplicate/case-alias keys and trailing JSON are
rejected. JSON requests are bounded to 64 KiB, configs to 32 KiB.

## Filesystem and commit boundary

The production path comes only from `FOLDERID_LocalAppData`, with shell path
verification disabled during lookup, then the fixed `GodUseVPN Ultra` /
`BrowserProfiles` names. Environment variables and UI paths do not override it.
SYSTEM/service identities, elevated and non-medium-integrity tokens are
refused. Non-Windows platforms explicitly return unsupported. `List` and `Load`
never create directories. The existing `localstate.AcquireDirectory` validates
the local fixed-disk namespace and pins ancestors without following reparse
points. The final directory is additionally opened, checked by handle and
matched to its expected final long path.

Every record is one bounded `p-<id>.ubp` append journal. Child opens use
`NtCreateFile` relative to the held directory handle, with `OBJ_DONT_REPARSE`,
non-directory/open-reparse options and no write or delete sharing. A fresh
create uses `FILE_CREATE` and must return `FILE_CREATED`. Files must be local
regular disk objects, non-reparse, have exactly one hard link, a trusted owner
and no write-capable ACE for principals other than this user, SYSTEM or local
Administrators. New files use a protected DACL restricted to those principals.
Existing permissions are checked, never rewritten. The directory is enumerated
by its handle and bounded to 128 product records; unexpected entries are
refused, not removed.

Update takes the exclusive write-held record handle, validates the full
journal and compares `expectedRevision` to SHA-256 of every existing byte. It
appends one length-bounded, hash-checked frame with its previous commit hash,
then syncs it; previous committed bytes are never rewritten. The logical
commit is a complete frame and footer. A partial tail preserves the previous
complete draft, returns a different full-file revision and
`recoveryNeeded:true`, and refuses further updates. An explicit matching
delete or saving the recovered draft as a new record is required. Malformed
frames fail closed. Journals are limited to 8 MiB and frames to 48 KiB. Earlier
non-secret revisions remain in the record until it is deleted.

Deletion uses `FileDispositionInfo` on that exact verified, still-held file
handle. No path is reopened for deletion and there is no recursive deletion,
directory sweep, migration or automatic truncation. A failed new create is
also cleaned up through its own held handle. A cancellation concurrent with a
durable write can return an error after commit; reloading determines the latest
revision, and a stale retry cannot overwrite it.

This is conflict and namespace protection, not authenticated storage against an
attacker already controlling the same user token or the machine's
Administrators. It does not encrypt public draft fields or guarantee physical
media durability after power loss. Real Windows ACL/share/NT behavior remains
an isolated VM validation item. Local tests use injected synthetic storage;
they do not access the product directory, token or real native launch gates.

## Pure verification

```text
go test ./internal/browserprofiles
go test ./cmd/godusevpn -run BrowserProfiles
go vet ./internal/browserprofiles ./cmd/godusevpn
```

The synthetic suite covers create/load/rename/delete, Chinese names, safe IDs,
stale and cross-manager revisions, write-held conflicts, cancellation/shutdown,
manual UA preservation, all supported engine selections, rejected secrets,
unknown fields, every truncated frame boundary, corruption, short writes,
sync failures, entropy failure, foreign files and directory/file size limits.
Windows-only tests inspect native policy values without invoking Windows APIs.
