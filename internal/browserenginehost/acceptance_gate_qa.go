//go:build browserengine_acceptance

package browserenginehost

// Compiled only into the explicitly selected VM QA package. The same native
// path still enforces ordinary token, authenticated installed GUI/helper, Job,
// sealed code, exact WFP and protocol readback. No frontend flag can enable it.
const nativeAcceptanceBuild = true
