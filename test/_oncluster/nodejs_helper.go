package oncluster

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// WriteNewSimpleIndexJS is used to replace the content of "index.js" of a Node JS function created by a test case.
// File content will cause the deployed function to, when invoked, return the value specified on `withBodyReturning`
// params, which is handy for test assertions.
func WriteNewSimpleIndexJS(t *testing.T, nodeJsFuncProjectDir string, withBodyReturning string) {
	indexJsContent := fmt.Sprintf(`
function invoke(context) {
  return { body: '%v' }
}
module.exports = invoke;
`, withBodyReturning)

	err := os.WriteFile(filepath.Join(nodeJsFuncProjectDir, "index.js"), []byte(indexJsContent), 0644)
	AssertNoError(t, err)
}
