package version

var Vers, Kver, Hash string

// FuncUtilsImage is the func-utils image that func runs in the cluster: the
// minor in development on main, X.Y on release-X.Y. Builds can override it
// (FUNC_UTILS_IMG in the Makefile). When release-X.Y is cut, the workflow
// update-func-utils-image.yaml moves main to the next minor; see
// docs/func-utils-image.md.
const FuncUtilsImage = "ghcr.io/knative/func-utils:1.24"
