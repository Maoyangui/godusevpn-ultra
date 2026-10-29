# Publication history review

Scope: read-only Git object/history and release-workflow review before the first
Ultra publication. No branch, commit, tag, remote repository, push or history
rewrite was created by this review. No candidate URL was contacted. Findings
below contain no credential values or blob contents.

## Object integrity

The audited HEAD was upstream baseline
`b51aed4fd0958a8abf461a74c453d432fca155d4`, with 155 reachable commits.

- `git fsck --no-reflogs` returned success. Its 30 diagnostic lines were dangling
  object notices, with no missing/corrupt-object diagnostic.
- `git rev-list --objects --missing=print HEAD`: 2,690 reachable objects, no
  missing marker.
- The same check with `--all`: 3,793 reachable objects, no missing marker.
- `git count-objects -v` reported three packs with 3,843 packed objects and eight
  garbage items. Directory inspection identified eight orphan `.idx` files with
  no corresponding `.pack`. These were left unchanged. This housekeeping issue
  did not make the checked published references unreadable.

Dangling objects and orphan indexes are not evidence that all stored content is
safe to publish. The content scan below covered HEAD reachability, not dangling
objects, reflogs or every other reference's text.

## Historical content

The HEAD-reachable inventory contained 1,353 unique blobs: 1,301 valid UTF-8 text
blobs and 52 binary/NUL/invalid-UTF-8 blobs outside this text scan. Text payloads
were read in memory using `git cat-file --batch`; no historical checkout was
created.

An initial heuristic pass produced 136 candidate classifications: 109 for
credential-bearing/subscription URL patterns and 27 for literal credential
assignment patterns. These numbers are **not a count of confirmed secrets**.
Candidates can repeat one value across historical versions and include source
syntax or synthetic redaction fixtures. The historical URL candidates include
non-fixture domains in documentation and deployment test scripts. Their validity,
ownership and current access were not tested. The original public provenance
does not establish that republishing them is safe.

The selected private-key, GitHub/cloud-token, full Azure resource identity,
subscription-GUID and remote-login-address patterns found no additional match.
This is a limited pattern scan, not proof that arbitrary credentials, encoded
content, binary screenshots or commit metadata are absent. Candidate output was
restricted to blob identifier, path and classification, never matched values.

## Recommended publication boundary

Publish an independently reviewed **current tree as a new root commit** in the
new Ultra repository. Keep the existing fork branch, all local history and the
upstream remote unchanged. A separate no-parent publication commit/ref can meet
this boundary; no history rewrite is needed. Inherit neither the 155 commits nor
unreviewed old tags/branches. Push only the explicitly chosen new publication ref
and its reviewed release tag; do not use `--all`, `--tags` or `--mirror`.

Retain the upstream attribution and baseline `b51aed4` in the README/NOTICE,
retain LICENSE and third-party notices, and keep the existing Go module path.
Starting a new publication history does not remove these obligations or replace
source availability for the distributed build.

The new root does not sanitize its tree. Before creating it:

1. Freeze and review the exact file/index list. Exclude local credentials, real
   VPS outputs, account/configuration data, logs, journals, dependencies and
   ignored build artifacts; inspect any intentionally retained binary assets and
   screenshots separately.
2. Re-run the current-source publication scanner and a credential-pattern pass
   against the final staged tree. The working-tree scan observed no production
   credential match; one broader literal match was in the redaction test fixture.
   No exact-value scan for privately supplied secrets was available to this
   reviewer. The final staged tree has not yet been created or certified here.
3. Check the new commit's metadata and reachable files, then verify that its
   parent list is empty and that the release tag points to it. Confirm the target
   remote explicitly. These are subsequent publication steps, not actions taken
   by this review.

## Workflow and checksum correction

The Windows beta workflow parsed as YAML 1.2 with unique keys. Its nine
PowerShell run blocks passed Windows PowerShell 5.1 AST parsing. Action references
are full commit hashes; their official GitHub commit pages existed at review
time. The release job alone grants `contents: write`, depends on the Windows
checks and only runs for the exact `v0.01-beta` push tag. Version/output/artifact
names were consistent. No remote workflow run was performed; runner tools,
dependency availability, execution time and actual release publication remain
unverified by this review.

A confirmed cross-platform failure was corrected in `build-ultra.ps1`: the ZIP
checksum sidecar previously used Windows `Set-Content`, which emits CRLF. A GNU
`sha256sum --check` fixture treated the CR as part of the filename and failed.
The writer now explicitly uses ASCII without a BOM and a single LF terminator.
Only future generated sidecars change; existing QA ZIPs and sidecars were not
rewritten. The workflow itself was left unchanged.

`checksum-sidecar.cjs` parses the real build source and selects only this one
writer expression. It executes that expression against a newly created inert
Temp file, checks the exact bytes (ASCII, no BOM/CR, final LF), then invokes GNU
`sha256sum --check`. It passed with Windows PowerShell 5.1 and the available
PowerShell 7 host. It does not execute the build, any product EXE, installer,
network request or existing ZIP.

Reproduce from the repository root with installed tool paths:

```text
node docs/ultra/qa/checksum-sidecar.cjs <powershell-executable> <GNU-sha256sum-executable>
```

This checksum test does not establish a successful GitHub release or installed
application behavior.
