# Local editing tool deviation

2026-10-19, during the frontend audit repair batch. A local text-edit command used the unqualified Windows `python` launcher (`@'…local Path.read_text/write_text replacements…'@ | python -`). The Windows Python install manager unexpectedly updated itself to 26.4, fetched the Python distribution index/signature, and installed Python 3.15.0 before executing that local script.

This was an unintended tool bootstrap/network action, outside the planned offline-only mock validation. It was reported immediately to the parent agent and disclosed to the user. The command edited only the owned local privacy JavaScript source; no application privacy probe, product, UAC, browser setting, registry mutation, VPN or cloud operation was invoked. No project/user payload was intentionally submitted by the editing script. This record does not assert that the bootstrap made no network requests; the tool output explicitly records the installer requests.

No global runtime uninstall, rollback or environment modification was attempted afterward. Subsequent work uses apply_patch, existing Node and Go tools. The unqualified `python`/`py` launcher is not used again. If Python is necessary, only the previously supplied existing bundled runtime may be used after confirming its fixed path; this batch does not require it.

No account, personal filesystem path, cloud identifier or credential is included in this record.
