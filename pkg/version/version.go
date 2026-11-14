package version

// Default is the fallback version used when no build-time version was
// injected (e.g. source builds that bypass the Makefile).
const Default = "v0.0.0+source"

var (
	Vers = Default // overwritten by ldflags when make actually has a describe
	Kver string
	Hash string
)

// FuncUtilsImage is the func-utils image that func runs in the cluster: the
// minor in development on main, X.Y on release-X.Y. Builds can override it
// (FUNC_UTILS_IMG in the Makefile). When release-X.Y is cut, the workflow
// update-func-utils-image.yaml moves main to the next minor; see
// docs/func-utils-image.md.
const FuncUtilsImage = "ghcr.io/knative/func-utils:1.25"
