package function

import (
	"testing"

	"knative.dev/pkg/ptr"
)

func Test_validateGit(t *testing.T) {

	tests := []struct {
		name         string
		git          Git
		mandatoryGit bool
		errs         int
	}{
		{
			"correct 'Git - only URL",
			Git{
				URL: ptr.String("https://myrepo/foo.git"),
			},
			true,
			0,
		},
		{
			"correct 'Git - URL + revision",
			Git{
				URL:      ptr.String("https://myrepo/foo.git"),
				Revision: ptr.String("mybranch"),
			},
			true,
			0,
		},
		{
			"correct 'Git - URL + context-dir",
			Git{
				URL:        ptr.String("https://myrepo/foo.git"),
				ContextDir: ptr.String("my-folder"),
			},
			true,
			0,
		},
		{
			"correct 'Git - URL + revision & context-dir",
			Git{
				URL:        ptr.String("https://myrepo/foo.git"),
				Revision:   ptr.String("mybranch"),
				ContextDir: ptr.String("my-folder"),
			},
			true,
			0,
		},
		{
			"incorrect 'Git - bad URL",
			Git{
				URL: ptr.String("foo"),
			},
			true,
			1,
		},
		{
			"incorrect 'Git - missing URL",
			Git{},
			true,
			1,
		},
		{
			"correct 'Git - not mandatory",
			Git{},
			false,
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateGit(tt.git, tt.mandatoryGit); len(got) != tt.errs {
				t.Errorf("validateGit() = %v\n got %d errors but want %d", got, len(got), tt.errs)
			}
		})
	}
}
