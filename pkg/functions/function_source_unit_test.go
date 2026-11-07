package functions

import (
	"testing"
)

// TestSource_Load ensures a func.yaml with build.source is read as written:
// url, revision and dir, with no migration and no warning involved.
func TestSource_Load(t *testing.T) {
	f, err := NewFunction("testdata/source")
	if err != nil {
		t.Fatal(err)
	}
	want := Source{URL: "https://example.com/alice/testfunc.git", Revision: "v1.2.0", Dir: "functions/testfunc"}
	if f.Build.Source != want {
		t.Errorf("expected source %+v, got %+v", want, f.Build.Source)
	}
	if f.SpecVersion != LastSpecVersion() {
		t.Errorf("expected the fixture at the latest spec version %q, got %q", LastSpecVersion(), f.SpecVersion)
	}
}

func Test_validateSource(t *testing.T) {

	tests := []struct {
		name   string
		source Source
		errs   int
	}{
		{
			"correct 'Source - only URL https",
			Source{
				URL: "https://myrepo/foo.git",
			},
			0,
		},
		{
			"correct 'Source - only URL scp",
			Source{
				URL: "git@myrepo:foo.git",
			},
			0,
		},
		{
			"correct 'Source - URL + revision",
			Source{
				URL:      "https://myrepo/foo.git",
				Revision: "mybranch",
			},
			0,
		},
		{
			"correct 'Source - URL + dir",
			Source{
				URL: "https://myrepo/foo.git",
				Dir: "my-folder",
			},
			0,
		},
		{
			"correct 'Source - URL + revision & dir",
			Source{
				URL:      "https://myrepo/foo.git",
				Revision: "mybranch",
				Dir:      "my-folder",
			},
			0,
		},
		{
			"incorrect 'Source - bad URL",
			Source{
				URL: "foo",
			},
			1,
		},
		{
			"correct 'Source - not mandatory",
			Source{},
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateSource(tt.source); len(got) != tt.errs {
				t.Errorf("validateSource() = %v\n got %d errors but want %d", got, len(got), tt.errs)
			}
		})
	}
}
