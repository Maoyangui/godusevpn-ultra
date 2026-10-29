# Installed-service authentication and ordinary-user compatibility

2026-10-19. This changes only the Ultra fork. The previous authentication path
unconditionally queried the peer's primary token before reading SCM evidence.
An ordinary GUI must not need administrator/debug rights to query a SYSTEM
service token. Microsoft's [OpenProcessToken contract](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-openprocesstoken)
documents cross-account privilege and token access checks. Existing rejection
tests and elevated/SYSTEM runs did not establish ordinary-user compatibility.

The installed branch now binds the named pipe to SCM's current running,
nonzero PID, own-process service type, exact service command and canonical
`LocalSystem` configuration. It holds the process handle, reads its actual
image, and requires that image to match the SCM command. Before sending it
rechecks process liveness, pipe PID, fresh SCM evidence, mode and image. This
branch does not call `OpenProcessToken`; it does not change token/process ACLs
or elevate the GUI. A local/domain account alias or Unicode lookalike is not
accepted as `LocalSystem`.

Only an explicitly absent service or a stopped service with PID zero may use
the foreground branch. That branch retains the mandatory elevated token,
original identity checks and fixed service-sibling code/directory pins.
Unreadable SCM, unknown/racing instances and a change between installed and
foreground modes fail closed. Token-query failure does not select a fallback.

The account name is **SCM configuration evidence**, not a queried runtime token
SID. Installed code/tree trust remains the existing installer/administrator
boundary; this patch does not add a binary publisher signature or new installed
code-byte attestation. The original foreground code pins remain in use.

Author and independent reviewer ran IPC tests and vet successfully. ROOT also
ran the IPC suite/vet and desktop production tests/vet. Contract cases cover
the actual shared authentication/Validate reader path, installed zero token
queries even when the token reader denies access, account/command/PID/image
changes, unreadable SCM, process/pipe changes and strict foreground failures.
See `IPC_SCM_AUTH_SECOND_REVIEW.md` for the independent review.

Real ordinary-user access to the pipe, SCM and the held process rights, followed
by an authenticated `GetState`, is still required. Neither these tests nor the
old qa.2 SYSTEM report prove it. This source change requires a new candidate;
the frozen qa.2 binaries and their truthful test receipts remain unchanged.
