package oci

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Test_validateLink ensures that the validateLink function disallows
// links which are absolute or refer to targets outside the given root.
func Test_validateLink(t *testing.T) {
	root := "testdata/test-links"

	tests := []struct {
		path  string // path of the file within test project root
		valid bool   // If it should be considered valid
		name  string // descriptive name of the test
	}{
		{"absoluteLink", false, "disallow absolute-path links"},
		{"a.lnk", true, "links to files within the root are allowed"},
		{"...validName.lnk", true, "allow links with target of dot prefixed names"},
		{"linkToRoot", true, "allow links to the project root"},
		{"b/linkToRoot", true, "allow links to the project root from within subdir"},
		{"b/linkToCurrentDir", true, "allow links to a subdirectory within the project"},
		{"b/linkToRootsParent", false, "disallow links to the project's immediate parent"},
		{"b/linkOutsideRootsParent", false, "disallow links outside project root and its parent"},
		{"b/c/linkToParent", true, "allow links up, but within project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(root, tt.path)
			info, err := os.Lstat(path) // filepath.Walk does not follow symlinks
			if err != nil {
				t.Fatal(err)
			}
			_, err = validateLink(root, path, info)
			if err == nil != tt.valid {
				t.Fatalf("expected %v, got %v", tt.valid, err)
			}
		})
	}

	// Run a windows-specific absolute path test
	if runtime.GOOS == "windows" {
		path := "testdata/test-links/absoluteLinkWindows"
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		_, err = validateLink(root, path, info)
		if err == nil {
			t.Fatal("absolute path should be invalid on windows")
		}
	}
}