# Interrupted installation and recovery

The beta installer supports fresh installations. It intentionally preserves a partial directory and does not overwrite an existing service or unknown files. An interrupted installation is not a successful installation.

1. Keep the complete trusted package, installed files and all recovery records. Do not delete backups or reset Windows Firewall to make an error disappear.
2. Close the Ultra UI and helper. Check the exact `godusevpn-ultra` service in Services; the original `godusevpn` service is a separate product. Do not identify a service by the executable filename alone.
3. Run `Install-Ultra.ps1 -WhatIf` from a complete trusted package to review its fixed target and integrity. This does not resume or repair an interrupted copy. A hash proves consistency, not the publisher's identity.
4. If the installed recovery tools are complete and pass their manifest/ACL checks, restore each original user's settings in that user's session. The timezone recovery CLI is `privacy timezone-report` followed by `privacy timezone-restore`; another administrator's account must not substitute for that user.
5. Run the installed `Uninstall-Ultra.ps1 -WhatIf` to review the cleanup. The real uninstaller uses one maintenance window for machine cleanup and the matching service removal. Failure preserves tools and records. It cannot restore a different user's HKCU or bypass an untrusted journal.
6. If tools are incomplete, hashes or ownership differ, or the service belongs to another location, stop automatic installation. Preserve a copy for inspection and use the verified Windows recovery baseline. Automatic takeover, forced policy restore and recursive deletion are not recovery steps.

After verified cleanup, archive the preserved installation separately before a fresh installation. Never move an active service's files or remove backups whose restoration is unconfirmed. This beta has no automatic resume/migration command. The detailed state in the error and the audit/recovery records determine the next step.
