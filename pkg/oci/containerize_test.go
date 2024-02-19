package oci

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Test_validatedLinkTaarget ensures that the function disallows
// links which are absolute or refer to targets outside the given root, in
// addition to the basic job of returning the value of reading the link.
func Test_validatedLinkTarget(t *testing.T) {
	root := "testdata/test-links"

	allRuntimes := []string{} // optional list of runtimes a test applies to

	testAppliesToCurrentRuntime := func(testRuntimes []string) bool {
		if len(testRuntimes) == 0 {
			return true // no filter defined.
		}
		for _, r := range testRuntimes {
			if runtime.GOOS == r {
				return true // filter defined, and current is defined.
			}
		}
		return false // filter defined; current not in set.
	}

	tests := []struct {
		path     string   // path of the file within test project root
		valid    bool     // If it should be considered valid
		target   string   // optional test of the returned value (target)
		name     string   // descriptive name of the test
		runtimes []string // only apply this test to the given runtime(s)
	}{
		{"absoluteLink", false, "", "disallow absolute-path links on linux", []string{"linux"}},
		{"absoluteLinkWindows", false, "", "disallow absolute-path links on windows", []string{"windows"}},
		{"a.lnk", true, "", "links to files within the root are allowed", allRuntimes},
		{"...validName.lnk", true, "", "allow links with target of dot prefixed names", allRuntimes},
		{"linkToRoot", true, "", "allow links to the project root", allRuntimes},
		{"b/linkToRoot", true, "", "allow links to the project root from within subdir", allRuntimes},
		{"b/linkToCurrentDir", true, "", "allow links to a subdirectory within the project", allRuntimes},
		{"b/linkToRootsParent", false, "", "disallow links to the project's immediate parent", allRuntimes},
		{"b/linkOutsideRootsParent", false, "", "disallow links outside project root and its parent", allRuntimes},
		{"b/c/linkToParent", true, "", " allow links up, but within project", allRuntimes},
		{"a.lnk", true, "./a.txt", "spot-check link target on linux", []string{"linux"}},
		{"a.lnk", true, ".\\a.txt", "spot-check link target on windows ", []string{"windows"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !testAppliesToCurrentRuntime(tt.runtimes) {
				return // The test has a runtime filter defined
			}

			path := filepath.Join(root, tt.path)
			info, err := os.Lstat(path) // filepath.Walk does not follow symlinks
			if err != nil {
				t.Fatal(err)
			}
			target, err := validatedLinkTarget(root, path, info)

			if err == nil != tt.valid {
				t.Fatalf("expected validity '%v', got '%v'", tt.valid, err)
			}
			if tt.target != "" && target != tt.target {
				t.Fatalf("expected target %q, got %q", tt.target, target)
			}
		})
	}

}
