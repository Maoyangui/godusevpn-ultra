# App-owned secret clipboard cleanup

This module never polls the system clipboard or clears arbitrary content. The
Windows backend creates its private, message-only owner window only when the
user explicitly copies a secret. Constructing the backend or reading its report
does not call clipboard APIs. Other operating systems report unsupported.

Timed cleanup is **off by default**. The separate per-user
`clipboard-privacy.json` file stores only `enabled` and `delaySeconds` (5–600).
It contains no secret, digest, sequence or service/network settings. A changed
delay applies to the next secret copy. Disabling or cancelling forgets the
tracked receipt and preserves current clipboard content.

When enabled, a successful native `CF_UNICODETEXT` write records the nonzero
Windows clipboard sequence number and a SHA-256 digest in memory. Cleanup opens
the clipboard and, while holding that same native lock, checks the sequence,
private owner window and content digest before calling `EmptyClipboard`. A
later copy is preserved even if the user copied identical text. Unverifiable
receipts are not scheduled. Clipboard busy retries are limited to three total
attempts; a failed cleanup retains its receipt for a manual conditional retry.
Exit attempts conditional cleanup of a still-tracked receipt, then closes the
native owner window. Busy/error exit cleanup is reported as failed and does not
interact with a tunnel or network settings.

The Go/Wails bridge accepts a subscription profile ID, reads its URL through the
existing authenticated API, and passes it directly to this module. It returns
only nonsecret status. Errors are fixed codes; raw source/backend errors and
secret URLs are not returned or logged. The UI exposes a distinct **Copy secret
link** button. Its ordinary log-copy helper is unchanged and is never tracked.
Clipboard preferences and operations have their own API, independent of
`SaveSettings` and connection changes.

This clears the current clipboard only. It cannot erase clipboard history,
cloud synchronization or copies already read by other software. It does not
claim to erase all Go/Windows memory copies. The UI help states these limits.

Validation uses fake backends, deterministic timers, temporary preference files
and an isolated JSDOM bridge. No product, browser, service or real clipboard has
been run in the local development session. A real Windows owner/sequence and
shutdown check remains a release/VPS validation requirement.

Official API contracts:

- [OpenClipboard](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-openclipboard)
- [SetClipboardData](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setclipboarddata)
- [GetClipboardSequenceNumber](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getclipboardsequencenumber)
- [Clipboard operations](https://learn.microsoft.com/en-us/windows/win32/dataxchg/clipboard-operations)
