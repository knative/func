package oci

import "testing"

// Test_isWithin ensures that the isWithin method checks
// that various combinations of parent and child paths.
func Test_isWithin(t *testing.T) {
	tests := []struct {
		parent string
		child  string
		want   bool
		name   string
	}{
		{"/", "/b", true, "Base case, a subdir of an absolute path"},
		{"/a", "/ab", false, "Ensure simple substring does not match"},
		{"./", ".", true, "Ensure links are both made absolute"},
		{"/a/b/../c", "/a/c/d/../", true, "Ensure the links are both sanitized"},
		{"/a", "/a/b/../../", false, "Ensure escaping the parent is a mismatch"},
		{"./", "../", false, "Ensure simple relative mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isWithin(tt.parent, tt.child)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Log(tt.name)
				t.Errorf("isWithin() = %v, want %v", got, tt.want)
			}
		})
	}

}
