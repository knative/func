//go:build e2elc

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"knative.dev/func/test/testhttp"

	common "knative.dev/func/test/common"
)

var runtimeSupportMap = map[string][]string{
	"node":       {"pack", "s2i"},
	"go":         {"pack", "s2i"},
	"rust":       {"pack"},
	"python":     {"pack", "s2i"},
	"quarkus":    {"pack", "s2i"},
	"springboot": {"pack"},
	"typescript": {"pack", "s2i"},
}

type lifecycleTestCase struct {
	Runtime string
	Builder string
}

// prepareTestMatrix creates a list of runtime x builder that will be part of the lifecycle test.
func prepareTestMatrix() (testCase []lifecycleTestCase) {
	targetBuilder, _ := os.LookupEnv("FUNC_BUILDER")
	runtimes, present := os.LookupEnv("E2E_RUNTIMES")
	var runtimeList = []string{}
	if present {
		if runtimes != "" {
			runtimeList = strings.Split(runtimes, " ")
		}
	} else {
		for k := range runtimeSupportMap {
			runtimeList = append(runtimeList, k)
		}
	}
	for _, r := range runtimeList {
		for _, supportedBuilder := range runtimeSupportMap[r] {
			if targetBuilder == "" || supportedBuilder == targetBuilder {
				testCase = append(testCase, lifecycleTestCase{r, supportedBuilder})
			}
		}
	}
	return
}

// TestFunctionCloudEventsTemplate will invoke a language runtime test against (by default) all supported runtimes.
// The Environment Variable E2E_RUNTIMES can be used to select the languages/runtimes to be tested
// The Environment Variable FUNC_BUILDER can be used to select the builder (s2i or pack).
func TestFunctionHttpTemplate(t *testing.T) {
	var testMatrix = prepareTestMatrix()
	for _, tc := range testMatrix {
		t.Run(fmt.Sprintf("%v_%v_test", tc.Runtime, tc.Builder), func(t *testing.T) {
			lifecycleHttpTest(t, tc.Runtime, tc.Builder)
		})
	}
}

func lifecycleHttpTest(t *testing.T, language string, builder string) {

	var funcName = "http-function-" + language + "-" + builder
	var funcPath = filepath.Join(t.TempDir(), funcName)

	knFunc := common.NewKnFuncShellCli(t)

	knFunc.Exec("create", "--language", language, "--template", "http", funcPath)
	knFunc.Exec("deploy", "--registry", common.GetRegistry(), "--builder", builder, "--path", funcPath)
	defer knFunc.Exec("delete", "--path", funcPath)

	_, functionUrl := common.WaitForFunctionReady(t, funcName)

	validator, ok := httpFuncValidatorMap[language]
	if ok {
		validator.InvokeAndAssert(t, functionUrl)
	}

}

// Basic function responsiveness Test Validator
type FuncResponsivenessValidator struct {
	urlMask         string
	method          string
	contentType     string
	bodyData        string
	expects         string
	customValidator func(statusCode int, responseBody string) error
}

func (f *FuncResponsivenessValidator) InvokeAndAssert(t *testing.T, functionUrl string) {
	targetUrl := fmt.Sprintf(f.urlMask, functionUrl)
	headers := testhttp.HeaderBuilder().AddNonEmpty("Content-Type", f.contentType).Headers

	statusCode, funcResponse := testhttp.TestUrl(t, f.method, f.bodyData, targetUrl, headers)

	if f.customValidator != nil {
		err := f.customValidator(statusCode, funcResponse)
		assert.NilError(t, err)
	} else {
		assert.Assert(t, statusCode == 200)
		assert.Assert(t, strings.Contains(funcResponse, f.expects), "Function response body does not contains %s", f.expects)
	}
}

var httpFuncValidatorMap = map[string]FuncResponsivenessValidator{
	"node": {
		urlMask: "%s?message=hello",
		expects: `{"message":"hello"}`,
	},
	"go": {
		urlMask: "%s?message=hello",
		expects: "message=hello",
	},
	"python": {
		urlMask: "%s",
		expects: `OK`,
	},
	"quarkus": {
		urlMask: "%s?message=hello",
		expects: `{"message":"hello"}`,
	},
	"springboot": {
		urlMask: "%s?message=hello",
		expects: "{message=hello}",
	},
	"typescript": {
		urlMask:     "%s",
		method:      "POST",
		contentType: "application/json",
		bodyData:    `{"message":"hello"}`,
		expects:     `{"message":"hello"}`,
	},
}
