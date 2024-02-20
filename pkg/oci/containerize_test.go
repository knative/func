package oci

import (
	"path/filepath"
	"runtime"
	"testing"
)

// Test_validatedLinkTaarget ensures that the function disallows
// links which are absolute or refer to targets outside the given root, in
// addition to the basic job of returning the value of reading the link.
func Test_validatedLinkTarget(t *testing.T) {
	root := "testdata/test-links"

	// Windows-specific absolute link and link target values:
	absoluteLink := "absoluteLink"
	linkTarget := "./a.txt"
	if runtime.GOOS == "windows" {
		absoluteLink = "absoluteLinkWindows"
		linkTarget = ".\\a.txt"
	}

	tests := []struct {
		path   string // path of the file within test project root
		valid  bool   // If it should be considered valid
		target string // optional test of the returned value (target)
		name   string // descriptive name of the test
	}{
		{absoluteLink, false, "", "disallow absolute-path links on linux"},
		{"a.lnk", true, linkTarget, "spot-check link target"},
		{"a.lnk", true, "", "links to files within the root are allowed"},
		{"...validName.lnk", true, "", "allow links with target of dot prefixed names"},
		{"linkToRoot", true, "", "allow links to the project root"},
		{"b/linkToRoot", true, "", "allow links to the project root from within subdir"},
		{"b/linkToCurrentDir", true, "", "allow links to a subdirectory within the project"},
		{"b/linkToRootsParent", false, "", "disallow links to the project's immediate parent"},
		{"b/linkOutsideRootsParent", false, "", "disallow links outside project root and its parent"},
		{"b/c/linkToParent", true, "", " allow links up, but within project"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(root, tt.path)
			target, err := validatedLinkTarget(root, path)

			if err == nil != tt.valid {
				t.Fatalf("expected validity '%v', got '%v'", tt.valid, err)
			}
			if tt.target != "" && target != tt.target {
				t.Fatalf("expected target %q, got %q", tt.target, target)
			}
		})
	}

}
