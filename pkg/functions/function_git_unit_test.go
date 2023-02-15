package functions

import (
	"testing"
)

func Test_validateGit(t *testing.T) {

	tests := []struct {
		name string
		git  Git
		errs int
	}{
		{
			"correct 'Git - only URL https",
			Git{
				URL: "https://myrepo/foo.git",
			},
			0,
		},
		{
			"correct 'Git - only URL scp",
			Git{
				URL: "git@myrepo:foo.git",
			},
			0,
		},
		{
			"correct 'Git - URL + revision",
			Git{
				URL:      "https://myrepo/foo.git",
				Revision: "mybranch",
			},
			0,
		},
		{
			"correct 'Git - URL + context-dir",
			Git{
				URL:        "https://myrepo/foo.git",
				ContextDir: "my-folder",
			},
			0,
		},
		{
			"correct 'Git - URL + revision & context-dir",
			Git{
				URL:        "https://myrepo/foo.git",
				Revision:   "mybranch",
				ContextDir: "my-folder",
			},
			0,
		},
		{
			"incorrect 'Git - bad URL",
			Git{
				URL: "foo",
			},
			1,
		},
		{
			"correct 'Git - not mandatory",
			Git{},
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateGit(tt.git); len(got) != tt.errs {
				t.Errorf("validateGit() = %v\n got %d errors but want %d", got, len(got), tt.errs)
			}
		})
	}
}
