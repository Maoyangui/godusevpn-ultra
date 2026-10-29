# Official browser version cache

This module implements official version discovery, bounded downloads, ZIP/7z
SFX data extraction, exact version checks and a complete SHA256 tree inventory.
It does not start a browser or installer. It is separate from the audited
WebView2 host and does not change that host's accepted engine/configuration.

```go
manager, err := browserengine.New(&http.Client{Transport: verifiedRouteTransport})
catalog, err := manager.RefreshCatalog(ctx) // explicit network action
downloaded, err := manager.DownloadVersion(ctx, browserengine.Chrome, version)
versions, err := manager.ListInstalled() // local complete-tree verification
version, err := manager.InspectInstalled(browserengine.Firefox, versionString)
plan, err := manager.Plan(browserengine.Firefox, versionString)
deleted, err := manager.DeleteVersion(ctx, browserengine.Firefox, versionString)
```

`New` requires an explicit HTTP transport and an ordinary Windows user. It
selects only `%LocalAppData%\GodUseVPN Ultra\BrowserEngines`; it does not accept
paths, binaries, flags or caller download URLs. No constructor makes requests
or creates directories. The caller authorizes network activity and supplies
its verified route. The module does not construct a default/direct transport
or recover from a failed proxy by retrying directly. Redirects and HTTP content
encoding are refused. Calls return fixed error identifiers without raw URLs,
paths, diagnostics or credentials.

Engine IDs are `chrome-for-testing` and `firefox`, platform `win64`. Chrome is
the real Google Chrome for Testing distribution, based on Chromium/Blink/V8;
it is not an independently built unbranded Chromium binary. Firefox is the
official Mozilla Firefox/Gecko distribution. The Firefox Windows installer is
read as an archive; only `core/` contents are extracted. Its installer code is
never executed, including silent installation. All bundled core files and
notices are retained.

A download creates a new version directory exclusively and writes its complete
receipt last. Existing directories are refused, including an incomplete prior
download. Incomplete directories are not reused, overwritten or automatically
deleted. Inspect refuses missing/modified/extra files, unknown directories,
links, malformed receipts and a different PE/resource/application.ini version.
The module confines writes using Go's `os.Root`, rejects ambiguous Windows
names, links/reparse archive members, duplicates/case collisions, traversal,
non-ASCII paths and expansion limits. Ordinary localstate leases pin the local
ancestor path; their existing same-user write-lease limitations still apply.

`TreeVerified` and `VersionVerified` describe cache checks. A same-user writable
receipt is not publisher authentication or an immutable code lease. Chrome's
official JSON supplies no archive digest; HTTPS origin, local archive digest
and later receipt checks are recorded separately. Mozilla's SHA512SUMS is
checked before parsing its SFX, fetched over the same official HTTPS channel;
this module does not verify a detached Mozilla PGP signature.

`CanLaunch` is always false, including in `Plan`. A separate ordinary-user
host needs immutable publisher/code identity, job/process lifecycle, independent
per-app no-direct enforcement, pre-navigation parameter/worker/permission
configuration and actual proxy/readback/failure tests. Proxy command-line flags
or Firefox preferences alone do not meet that requirement. Android/iOS/macOS/
Windows webpage environment emulation does not provide another native OS or an
iOS/Safari engine. Those features are not implemented by this cache.

Run `go test ./internal/browserengine` and `go vet ./internal/browserengine`.
Default tests use fake HTTP transports and temporary inert ZIP/SFX data, with
no sockets or browser processes. The two `TestManualOfficial*` cases skip by
default; explicit cached official artifacts can enable static archive-only
review. The evidence and exact commands are in
`docs/ultra/qa/BROWSER_ENGINE_REVIEW_2026-10-26.md`. `third_party/` retains the
new/updated extraction dependency licenses; release packaging must retain these
notices and provide the fixed corresponding source versions.

QA12 adds verified-version cache deletion with cross-process operation and
version-use locks, exact native object handles, controller start/delete
serialization and truthful partial-failure receipts. It never deletes browser
profiles/cookies or session instances. See
`docs/ultra/qa/BROWSER_ENGINE_DELETE_QA12_2026-10-27.md` for scope, source/TEMP
tests and remaining isolated-VM acceptance. Session instances do not contain
cache lock files and retain the guard's exact directory membership contract.
