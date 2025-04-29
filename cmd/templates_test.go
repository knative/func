package cmd

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gotest.tools/v3/assert"
	. "knative.dev/func/pkg/testing"
)

// TestTemplates_Default ensures that the default behavior is listing all
// templates for all language runtimes.
func TestTemplates_Default(t *testing.T) {
	_ = FromTempDirectory(t)

	buf := piped(t) // gather output
	cmd := NewTemplatesCmd(NewClient)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	expected := `LANGUAGE     TEMPLATE
go           cloudevents
go           http
node         cloudevents
node         http
python       cloudevents
python       http
quarkus      cloudevents
quarkus      http
rust         cloudevents
rust         http
springboot   cloudevents
springboot   http
typescript   cloudevents
typescript   http`

	if d := cmp.Diff(expected, buf()); d != "" {
		t.Error("output missmatch (-want, +got):", d)
	}
}

// TestTemplates_JSON ensures that listing templates respects the --json
// output format.
func TestTemplates_JSON(t *testing.T) {
	_ = FromTempDirectory(t)

	buf := piped(t) // gather output
	cmd := NewTemplatesCmd(NewClient)
	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	expected := `{
  "go": [
    "cloudevents",
    "http"
  ],
  "node": [
    "cloudevents",
    "http"
  ],
  "python": [
    "cloudevents",
    "http"
  ],
  "quarkus": [
    "cloudevents",
    "http"
  ],
  "rust": [
    "cloudevents",
    "http"
  ],
  "springboot": [
    "cloudevents",
    "http"
  ],
  "typescript": [
    "cloudevents",
    "http"
  ]
}`

	if d := cmp.Diff(expected, buf()); d != "" {
		t.Error("output missmatch (-want, +got):", d)
	}
}

// TestTemplates_ByLanguage ensures that the output is correctly filtered
// by language runtime when provided.
func TestTemplates_ByLanguage(t *testing.T) {
	_ = FromTempDirectory(t)

	cmd := NewTemplatesCmd(NewClient)
	cmd.SetArgs([]string{"go"})

	// Test plain text
	buf := piped(t)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	expected := `cloudevents
http`

	output := buf()
	if output != expected {
		t.Fatalf("expected plain text:\n'%v'\ngot:\n'%v'\n", expected, output)
	}

	// Test JSON output
	buf = piped(t)
	cmd.SetArgs([]string{"go", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	expected = `[
  "cloudevents",
  "http"
]`

	output = buf()
	if output != expected {
		t.Fatalf("expected JSON:\n'%v'\ngot:\n'%v'\n", expected, output)
	}

}

func TestTemplates_ErrTemplateRepoDoesNotExist(t *testing.T) {
	_ = FromTempDirectory(t)

	cmd := NewTemplatesCmd(NewClient)
	cmd.SetArgs([]string{"--repository", "https://github.com/boson-project/repo-does-not-exist"})
	err := cmd.Execute()
	assert.Assert(t, err != nil)
	assert.Assert(t, errors.Is(err, ErrTemplateRepoDoesNotExist))
}

func TestTemplates_WrongRepositoryUrl(t *testing.T) {
	_ = FromTempDirectory(t)

	cmd := NewTemplatesCmd(NewClient)
	cmd.SetArgs([]string{"--repository", "wrong://github.com/boson-project/repo-does-not-exist"})
	err := cmd.Execute()
	assert.Assert(t, err != nil)
}
