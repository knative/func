package cmd

import (
	"testing"
)

// TestLanguages_Default ensures that the default behavior of listing
// all supported languages is to print a plain text list of all the builtin
// language runtimes.
func TestLanguages_Default(t *testing.T) {
	_ = fromTempDirectory(t)

	buf := piped(t) // gather output
	cmd := NewLanguagesCmd(NewClient)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	expected := `go
node
python
quarkus
rust
springboot
typescript`
	output := buf()
	if output != expected {
		t.Fatalf("expected:\n'%v'\ngot:\n'%v'\n", expected, output)
	}
}

// TestLanguages_JSON ensures that listing languages in --json format returns
// builtin languages as a JSON array.
func TestLanguages_JSON(t *testing.T) {
	_ = fromTempDirectory(t)

	buf := piped(t) // gather output
	cmd := NewLanguagesCmd(NewClient)
	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	expected := `[
  "go",
  "node",
  "python",
  "quarkus",
  "rust",
  "springboot",
  "typescript"
]`
	output := buf()
	if output != expected {
		t.Fatalf("expected:\n%v\ngot:\n%v\n", expected, output)
	}
}
