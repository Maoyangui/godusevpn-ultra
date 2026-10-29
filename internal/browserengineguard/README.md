# Browser session guard — native implementation awaiting VM acceptance

This package uses its own WFP provider/sublayer, separate from the VPN and
general appfirewall. The privileged fixed helper constructs it with its held,
authenticated ordinary GUI process handle. The user's SID and LocalAppData
come from that token, including alternate-administrator UAC: the token requests
QUERY|IMPERSONATE as required by SHGetKnownFolderPath, uses DONT_VERIFY, then
rejects UNC/device/ADS/ambiguous paths and unconfirmed/network/SUBST drives
before any administrator filesystem access.

Alternate-account parent access first uses the existing exact process mask.
Only access-denied permits one fixed same-mask retry on a locked OS thread
using a verified private impersonation-token copy with SeDebug enabled solely
on that copy. Revert plus explicit no-thread-token readback must precede
unlocking or delivery of the new process handle. Dirty threads exit locked;
unconfirmed retirement refuses the capability. This QA12 source path has pure
failure/order tests, but its native behavior still needs VM acceptance. See
[scoped parent-access evidence and contract](../../docs/ultra/qa/BROWSER_GUARD_SCOPED_DEBUG_QA12_2026-10-27.md).

Install duplicates the GUI's Job handle, verifies kill-on-close/no-breakaway,
single initial main PID, creation time and the fixed session-instance tree. The
trusted host must create that main process suspended and call Install before
ResumeThread. The helper's single-PID check is not an independent measurement
of the thread's suspended state. It pins and checks every instance file and
derives all EXE identities. Exact persistent BLOCK filters leave only the
session's IPv4 loopback TCP relay endpoint; Firefox also needs the narrow
loopback incoming BiDi tuple. Installation is followed by separate native
readback of owner, keys, flags, metadata and every condition. There are no broad
PERMIT filters, no VPN filter mutations, and no blanket namespace deletion.

Remove independently queries Job ActiveProcesses and deletes only an intact
owned set after the count is zero, then checks absence. Peer-loss Close first
terminates the Job and queries zero. Unknown/active state retains filters.
The helper's duplicate Job handle affects last-handle-close behavior, so it
actively terminates on GUI loss instead of claiming GUI handle closure alone
already killed the tree.

The provider currently has no associated SCM service name. Persistent filters
cover helper-process exit, but Microsoft documents that a provider without an
auto-start service association may have its filters disabled when BFE starts.
This implementation detects the provider's DISABLED flag as unknown. The
existing VPN guard documents real observations where the nil-service design
survived restart; this independent provider still requires its own VM evidence.
It preserves that established design and does not install a new resident
service or claim BFE-restart acceptance from local pure tests.

No WFP/Job/browser operations were run by local tests. Static compilation,
vet, pure namespace/fake-lifecycle and synthetic-descriptor tests pass; native
ABI, classification, lifecycle and
traffic remain VM acceptance items. Job membership does not itself forbid
arbitrary system-image child processes. Code sealing is implemented through
the pinned handles: Administrators owner, protected exact SYSTEM/admin full
and originating user RX DACL, followed by full re-enumeration and same-handle
descriptor/final-path readback. The writable profile is a separate leaf. Its
real sharing/ACL behavior still requires VM evidence. Windows amd64/arm64
ordinary builds expose the same native Start path as the acceptance package;
the helper/Job/WFP checks are unchanged and no frontend bypass is available.
This source integration still requires controlled VM evidence for real browser,
proxy and traffic behavior before it is described as fully qualified.

Official contracts:
[SHGetKnownFolderPath](https://learn.microsoft.com/en-us/windows/win32/api/shlobj_core/nf-shlobj_core-shgetknownfolderpath),
[FWPM_PROVIDER0 BFE/service behavior](https://learn.microsoft.com/en-us/windows/win32/api/fwpmtypes/ns-fwpmtypes-fwpm_provider0),
[WFP filtering conditions](https://learn.microsoft.com/en-us/windows/win32/fwp/filtering-condition-identifiers-),
[Job objects](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects).
