# QA12 source: delete one downloaded browser version

This records source/TEMP/fake tests, not VM acceptance. QA11 ZIP is unchanged.
No user cache, browser, installer, WFP rule, service or VM was run or deleted.

## API and scope

`App.BrowserEngineDelete(engineId, version)` returns
`{engineId,version,deleted,files,bytes}`. `deleted` becomes true only after all
selected objects are disposed and closed and the exact target is confirmed
absent inside the same exclusive store lease. Counts include the receipt and
logical file lengths, not filesystem free space. Errors have fixed text without
raw paths, arguments, credentials or file contents.

`Manager.DeleteVersion` accepts only the existing engine/version grammars and
the ordinary user's fixed `%LocalAppData%\GodUseVPN Ultra\BrowserEngines` cache.
Its resolved absolute target must be exactly `<cache>/<engine>/<version>` before
I/O. Native localstate acquisition rejects UNC/device/ADS/ambiguous paths,
non-fixed drive aliases and parent reparse points before descending. Session
instance Managers do not have deletion authority. No path is supplied by Wails.
Instances, profiles, cookies, engine parents and neighboring versions are never
enumerated for cleanup or removed.

The complete version must pass existing official URL/distribution, receipt,
full tree SHA256 and PE version checks. Missing receipts, incomplete/corrupt
downloads, extra members and nonregular members refuse deletion. An absent
version returns not-installed, never invented success. A user-writable receipt
remains a cache integrity record, not publisher authentication.

## Concurrency

Controller maintenance reserves the same mutex-protected slot as Begin before
filesystem I/O. Any tracked owner blocks deletion: starting with no returned
session, cancelling, running, pending close, close error, or unknown process or
filter cleanup. During maintenance, Begin and another delete reject. The mutex
is released during filesystem work, so Status remains nonblocking. Failure
releases only maintenance; it never drops a tracked native owner.

Cache Download/Inspect/List/Delete now share a permanent zero-byte operation
file with an exclusive, fail-fast Windows `LockFileEx` lock. This coordinates
different Managers and processes. List inspects one protected snapshot without
nested lock acquisition. Parent directory pins alone are not an operation lock.

Each native session takes a shared per-version usage file lock before its first
session download. CodeLease holds it through confirmed process/filter cleanup.
Delete needs the same version's exclusive lock. Unknown filter removal retains
code/profile leases in the existing strong owner registry: a closed Done channel
alone does not release the version. Permanent lock leaves must not be unlinked
while a process can hold them, which would split the lock namespace. They are
created only in BrowserEngines, never in the guard's exact session-instance tree.
Session roots retain their existing random identity/exclusive creation and root
validation instead. Locks end when their owning process exits; this is not a
durable cross-reboot cleanup database or a claim that persistent WFP rules were
removed after a crash.

## Exact objects and incomplete deletion

The native backend opens the engine parent and every selected object relative
to held directory handles using `NtCreateFile(FILE_OPEN)`, `OBJ_DONT_REPARSE`,
`FILE_OPEN_REPARSE_POINT`, and no write/delete sharing. Only selected objects
receive DELETE access. Results require valid handles, successful IO status and
FILE_OPENED. Nothing is created or overwritten by deletion preparation.

Type, no reparse, single hardlink, length and SHA256 are checked on the same
handles later used for disposition. The persisted receipt's complete canonical
JSON must equal the inspected receipt, rejecting unknown/case-alias fields.
Exact directory membership is checked after all known members are pinned.

Remove then sets `FileDispositionInfo` on those handles: files first, receipt
last among files, directories deepest first, version directory last. It never
uses absolute RemoveAll, reopens a changed path, or removes an unknown member.
An externally added file can prevent an empty directory and therefore cause
failure; deletion does not expand its scope to remove it.

Sharing/disposition/close/cancellation/absence-check failures never report
success. Remaining members are preserved. After partial actions, a best-effort
bounded CREATE_NEW marker in the cache root records engine/version, action
counts, time and fixed incomplete state. Existing markers are not followed or
overwritten. Later inspection/deletion refuses the incomplete tree. Automatic
recovery or broad removal of unidentified leftovers is outside this API.

## Verification and remaining VM acceptance

Go tests use fake HTTP and inert TEMP archives. Windows TEMP lock tests exercise
actual independent LockFileEx handles, shared/exclusive conflicts, release-once
and malformed leaves, without opening user engine directories. Native deletion
tests use in-memory fake handles, never real NtCreateFile/deletion calls: flags,
ABI/status, exact membership, reparse/hardlink rejection, altered/extra receipts,
reader conflict, hashes, partial failure, cancellation and Remove/Close races.

Manager tests cover two versions and sibling profile/instance sentinels, absent
vs corrupt receipts, false success, partial markers, different Managers during
download, multiple users of one version, startup download failure release and
no cache lock contamination of session roots. Controller tests cover retained
states, maintenance vs Begin/delete, responsive Status and failure release.
Normal and acceptance-tag host tests/vet and desktop-production controller tests
are required; DOM tests remain mock interaction evidence.

In an isolated VM, download two official versions, close one, then delete it
through the real GUI. Independently confirm exact absence, neighbor hashes and
unchanged profile/cookie stores. Repeat with active/starting/cancelling/unknown
owners and another process holding a usage lease: refusal is required. A reader
that denies delete sharing must refuse before mutation. Finally force a bounded
directory-disposition failure and verify no success receipt or neighboring
removal. Native deletion sharing/disposition behavior remains unverified until
those real VM tests are performed.
