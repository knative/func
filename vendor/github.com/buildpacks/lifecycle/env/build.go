package env

import (
	"runtime"
	"strings"
)

var BuildEnvIncludelist = []string{
	"CNB_STACK_ID",
	"HOSTNAME",
	"HOME",
	"HTTPS_PROXY",
	"https_proxy",
	"HTTP_PROXY",
	"http_proxy",
	"NO_PROXY",
	"no_proxy",
}

var ignoreEnvVarCase = runtime.GOOS == "windows"

// NewBuildEnv returns an build-time Env from the given environment.
//
// Only keys in the BuildEnvIncludelist will be added to the Environment.
func NewBuildEnv(environ []string) *Env {
	return &Env{
		RootDirMap: POSIXBuildEnv,
		Vars:       varsFromEnv(environ, ignoreEnvVarCase, isNotIncluded),
	}
}

func matches(k1, k2 string) bool {
	if ignoreEnvVarCase {
		k1 = strings.ToUpper(k1)
		k2 = strings.ToUpper(k2)
	}
	return k1 == k2
}

func isNotIncluded(k string) bool {
	for _, wk := range BuildEnvIncludelist {
		if matches(wk, k) {
			return false
		}
	}
	for _, wks := range POSIXBuildEnv {
		for _, wk := range wks {
			if matches(wk, k) {
				return false
			}
		}
	}
	return true
}

var POSIXBuildEnv = map[string][]string{
	"bin": {
		"PATH",
	},
	"lib": {
		"LD_LIBRARY_PATH",
		"LIBRARY_PATH",
	},
	"include": {
		"CPATH",
	},
	"pkgconfig": {
		"PKG_CONFIG_PATH",
	},
}
