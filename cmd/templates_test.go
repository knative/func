package cmd

import (
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	fn "knative.dev/func"
)

// TestTemplates_Default ensures that the default behavior is listing all
// templates for all language runtimes.
func TestTemplates_Default(t *testing.T) {
	_ = fromTempDirectory(t)

	buf := piped(t) // gather output
	cmd := NewTemplatesCmd(NewClientFactory(func() *fn.Client {
		return fn.New()
	}))
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
	output := buf()
	if output != expected {
		t.Fatalf("expected:\n'%v'\n\ngot:\n'%v'\n", expected, output)
	}
}

// TestTemplates_JSON ensures that listing templates respects the --json
// output format.
func TestTemplates_JSON(t *testing.T) {
	_ = fromTempDirectory(t)

	buf := piped(t) // gather output
	cmd := NewTemplatesCmd(NewClientFactory(func() *fn.Client {
		return fn.New()
	}))
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

	output := buf()
	for i, c := range expected {
		if len(output) <= i {
			t.Fatalf("output missing character(s) '%v', '%s' and later\n", i, string(c))
		}
		if rune(output[i]) != c {
			t.Fatalf("Character at index %v expected '%s', got '%s'\n", i, string(c), string(output[i]))
		}
	}

	if output != expected {
		t.Fatalf("expected:\n%v\ngot:\n%v\n", expected, output)
	}
}

// TestTemplates_ByLanguage ensures that the output is correctly filtered
// by language runtime when provided.
func TestTemplates_ByLanguage(t *testing.T) {
	_ = fromTempDirectory(t)

	cmd := NewTemplatesCmd(NewClientFactory(func() *fn.Client {
		return fn.New()
	}))

	// Test plain text
	buf := piped(t)
	cmd.SetArgs([]string{"go"})
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
	_ = fromTempDirectory(t)

	cmd := NewTemplatesCmd(NewClientFactory(func() *fn.Client {
		return fn.New()
	}))
	cmd.SetArgs([]string{"--repository", "https://github.com/boson-project/repo-does-not-exist"})
	err := cmd.Execute()
	assert.Assert(t, err != nil)
	assert.Assert(t, errors.Is(err, ErrTemplateRepoDoesNotExist))
}

func TestTemplates_WrongRepositoryUrl(t *testing.T) {
	_ = fromTempDirectory(t)

	cmd := NewTemplatesCmd(NewClientFactory(func() *fn.Client {
		return fn.New()
	}))
	cmd.SetArgs([]string{"--repository", "wrong://github.com/boson-project/repo-does-not-exist"})
	err := cmd.Execute()
	assert.Assert(t, err != nil)
}
