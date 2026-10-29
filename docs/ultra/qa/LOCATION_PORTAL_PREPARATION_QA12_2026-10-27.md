# Windows Device Portal preparation — QA12 source checkpoint

The fixed helper now exposes three authenticated operations:
`LocationPortalPreparationReport`, `LocationPortalPreparationPrepare`, and
`LocationPortalPreparationRestore`. They accept no parameters. A response has
the bounded public `report` plus an optional private in-memory `config` only
when every component, service, gate, certificate and location-portal check has
passed. The config is never returned by the public App methods or JSON report.

Preparation is asynchronous. It creates the durable machine transaction before
the first system mutation, serializes the operation with the machine store and
the independent gate transaction, and returns a nonblocking `preparing` or
`restoring` report while DISM/service work is in progress. A generation check
prevents a late operation from publishing after helper close or a new operation.
Unknown, external, pending, reboot-required, or partial states retain the
backup and gate and refuse a new mutation.

The fixed Windows registry, SCM and Device Portal APIs do not provide one
atomic compare-and-set spanning the registry, service state and HTTP.sys. Each
step performs a fresh expected-state read immediately before its mutation and
a fresh readback afterward; a concurrent writer can still race that pair. Such
post-state mismatches are reported as `external_conflict` and retained for
manual recovery. This is a bounded observation contract, not a claim of global
system transaction atomicity.

The supported system actions are deliberately fixed: query or add/remove
`Tools.DeveloperMode.Core~~~~0.0.1.0` through the Windows DISM API, inspect the
fixed `WebManagement`, `SshBroker`, and `SshProxy` services, set only the
documented WebManagement DWORDs (`UseDynamicPorts=0`, `HttpPort=58080`,
`HttpsPort=58443`, `UseDefaultAuthorizer=1`), run the fixed
`%WINDIR%\System32\WebManagement.exe -Credentials ultralocation <random>`
command, start/stop only `WebManagement`, then verify the exact loopback TLS
certificate and the portal client. No arbitrary source path, executable, URL,
service name, registry path, or command flag crosses the IPC boundary.

The random portal password is bound to owner SID, transaction ID, and the final
certificate pin. It is stored only as a machine-scoped DPAPI encrypted private
credential box and exists in plaintext only in the authenticated helper's
memory and the private internal wire response. DPAPI associated-data mismatch,
bad schema, wrong owner/transaction/pin, malformed ciphertext, and oversized
boxes all refuse. The public report and machine journal contain no plaintext
credential or certificate DER.

Restore requires the fresh credential box to prove the current portal override
is disabled. If the override is enabled or unreadable, the helper leaves the
portal, component and gate in place and reports `location_restore_required`.
It then disables/stops the service, confirms the service state, removes only
the owned gate, restores the original fixed registry values, and verifies the
post-state before marking the transaction `restored`. A lost helper with an
unreadable DPAPI box is therefore a deliberate refusal requiring a new
authenticated preparation, not an automatic stop or uninstall.

The machine gate is the separate `locationportalgate` implementation. It is
associated with the installed GodUseVPN Ultra AutoStart service and must be
restored before that service is uninstalled. It is a four-filter inbound
loopback/port restriction; Microsoft documents that Device Portal also uses
DNS-SD discovery and that the Developer Mode FoD includes remote-development
components. No claim is made that four TCP filters suppress all discovery
broadcasts or every FoD side effect. The source checks newly present SSH
services remain disabled/stopped and reports the discovery boundary for VM
review.

Pure tests cover DISM parameter allowlists, all formal capability states,
pending/reboot/reload handling, cancellation while a native call is in flight,
session cleanup poisoning, DPAPI schema/owner/transaction/pin binding, and
public credential redaction. Package tests and vet pass without loading DISM,
SCM, registry, HTTP.sys, WFP, WebManagement, or a browser locally. Win10 VM
acceptance must still exercise a clean FoD add, service/configuration/readback,
HTTPS pin, location override guard, restore, and reboot/service-gate behavior.

Official contracts: [Windows Device Portal overview](https://learn.microsoft.com/en-us/windows/advanced-settings/device-portal),
[Desktop Device Portal configuration](https://learn.microsoft.com/en-us/windows/advanced-settings/device-portal-desktop),
[DismInitialize](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/dism/disminitialize-function?view=windows-11),
[DismAddCapability](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/dism/dismaddcapability?view=windows-11),
[DismCapabilityInfo](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/dism/dismcapabilityinfo?view=windows-11),
[Features on Demand](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/features-on-demand-non-language-fod?view=windows-11),
and [custom Device Portal SSL certificates](https://learn.microsoft.com/en-us/windows/advanced-settings/device-portal-ssl).
