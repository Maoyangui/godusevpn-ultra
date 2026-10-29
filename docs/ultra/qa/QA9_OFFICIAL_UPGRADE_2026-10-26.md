# QA.9 official upgrade: disposable Windows VM

Observed on 2026-10-26, Windows 11 Pro, x64 Windows PowerShell 5.1. This records installation behavior, not browser or network acceptance.

Candidate ZIP: `d8dc35c504e1a5c754ca7a3080f793769bfb69443c4151f1558ceff37f4b9095`, 54,799,224 bytes. The staged and installed package were checked against the pinned descriptor, manifest and all 1,958 payload entries. GUI SHA-256: `0434c1d41fe76b329e4648033d9b55686122f9130f46da764ca7933835ed2ea6`.

The first upgrade attempt refused during preflight: the disposable QA parent directory inherited ordinary-user permissions. The candidate stage itself was private. No uninstall or package replacement occurred in that attempt. A separate fixed-path script backed up the parent descriptor and tightened only that parent's DACL; an independent readback confirmed protected SYSTEM/Administrators access, unchanged owner/group, a private stage, and the old QA.8 LocalSystem service still running.

The unchanged upgrade script then reported:

1. `preflight_verified` and `qa9_package_verified`.
2. `qa8_install_verified`: exact QA.8 installed payload and Running/LocalSystem service.
3. `official_qa8_uninstalled`: the installed official uninstaller, with no validation bypass, exited successfully and the service was absent.
4. `qa8_all_files_retained`: the entire old installation was moved inside the disposable QA stage; all files and empty directories matched the prior snapshot.
5. `official_qa9_default_install_started`: cold official installer, **without `-Source`**.
6. `official_upgrade_verified`: version `0.01-beta-qa.9`, complete installed payload match, Running/LocalSystem service, retained old tree unchanged, `defaultSource=true`.

The private upgrade script was independently reviewed and pinned at SHA-256 `a14f1d493e6f9c08036f2300920de384a962d325eda13a8107ef42f4e4f0fb5d`. The fixed-root ACL script was pinned at `69a961c3a399a7a74868c420310d5b414632675a68fb33491153f646d92d3bf7`. Scripts, raw console captures and cloud identifiers remain outside the publication tree.

The normal-user remote desktop separately reported `S-1-16-8192` (Medium integrity). This establishes the test session's level; GUI operations, profile persistence, UAC, real browser UA/traffic, metadata execution and cleanup still require their own evidence.

QA.9 is immutable. CPU configuration, the revised modern UA generators, and subsequent cancellation/error-path changes belong to a later candidate and do not inherit this package's installation evidence.
