# Fixed Windows Device Portal preparation

This package is the privileged, authenticated preparation path for the local
Windows Device Portal used by the location test backend. It does not claim to
be a Windows GeolocationProvider, does not write a coordinate, and does not
change GNSS/IP location or unrelated firewall rules.

Only the three helper methods with no parameters are accepted:

* `LocationPortalPreparationReport`
* `LocationPortalPreparationPrepare`
* `LocationPortalPreparationRestore`

Preparation uses the fixed Developer Mode capability, fixed WebManagement SCM
service and registry values, the fixed 58080/58443 loopback endpoints, the
independently verified locationportalgate, and an exact TLS certificate pin.
The service executable is pinned to `%WINDIR%\System32\WebManagement.exe` and
`LocalSystem`; no arbitrary executable, source path, service name, registry
path, URL or flag is accepted from IPC.

State is journaled under the already trusted elevated machine privacy store and
serialized with an exclusive Windows file lock. The journal contains no
plaintext credentials. The random portal credential is a DPAPI
`CRYPTPROTECT_LOCAL_MACHINE` box bound to the originating GUI SID, transaction
ID and certificate pin. Plaintext exists only in helper memory and the private
authenticated response DTO; `locationportal.Config.MarshalJSON` remains
redacted.

Restore first reads the fresh portal override through that credential. An
enabled or unreadable override leaves the portal, component and gate in place.
Only after `WebManagement` is disabled/stopped and confirmed does it remove the
owned gate and restore the previous values. Reboot, pending DISM, foreign
service/filter state, journal corruption, partial cleanup, or uncertain reads
return a bounded refusal and retain recovery state.

Registry, SCM and HTTP.sys have no cross-subsystem compare-and-set. Each
mutation uses a fresh expected-state check and a fresh readback; concurrent
external changes can still race that pair and are retained as
`external_conflict` rather than overwritten or reported as success.

Pure tests use fake DISM, DPAPI, store, gate and backend delegates. They do not
load DISM, SCM, registry, HTTP.sys, WFP, WebManagement or browsers. Native
acceptance remains a separate VM task. Device Portal is a developer tool and
its official docs describe DNS-SD discovery and additional remote-development
components; the four TCP gate filters do not claim to suppress those effects.

Official sources: [Windows Device Portal overview](https://learn.microsoft.com/en-us/windows/advanced-settings/device-portal),
[Desktop configuration](https://learn.microsoft.com/en-us/windows/advanced-settings/device-portal-desktop),
[DismAddCapability](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/dism/dismaddcapability?view=windows-11),
[DismCapabilityInfo](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/dism/dismcapabilityinfo?view=windows-11),
[Features on Demand](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/features-on-demand-non-language-fod?view=windows-11),
and [Device Portal SSL certificates](https://learn.microsoft.com/en-us/windows/advanced-settings/device-portal-ssl).
