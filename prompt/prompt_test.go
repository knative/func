package prompt_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/boson-project/faas/prompt"
)

// TestForStringLabel ensures that a string prompt with a given label is printed to stdout.
func TestForStringLabel(t *testing.T) {

	var out bytes.Buffer
	var in bytes.Buffer
	in.Write([]byte("\n"))

	// Empty label
	_ = prompt.ForString("", "",
		prompt.WithInput(&in), prompt.WithOutput(&out))
	if string(out.Bytes()) != ": " {
		t.Fatalf("expected output to be ': ', got '%v'\n", string(out.Bytes()))
	}

	out.Reset()
	in.Reset()

	in.Write([]byte("\n"))

	// Populated lable
	_ = prompt.ForString("Name", "",
		prompt.WithInput(&in), prompt.WithOutput(&out))
	if string(out.Bytes()) != "Name: " {
		t.Fatalf("expected 'Name', got '%v'\n", string(out.Bytes()))
	}
}

// TestForStringLabelDefault ensures that a default, only if provided, is appended
// to the prompt label.
func TestForStringLabelDefault(t *testing.T) {
	var out bytes.Buffer
	var in bytes.Buffer
	in.Write([]byte("\n")) // [ENTER]

	// No lablel but a default
	_ = prompt.ForString("", "Alice",
		prompt.WithInput(&in), prompt.WithOutput(&out))
	if string(out.Bytes()) != "(Alice): " {
		t.Fatalf("expected '(Alice): ', got '%v'\n", string(out.Bytes()))
	}

	out.Reset()
	in.Reset()
	in.Write([]byte("\n")) // [ENTER]

	// Label with default
	_ = prompt.ForString("Name", "Alice",
		prompt.WithInput(&in), prompt.WithOutput(&out))
	if string(out.Bytes()) != "Name (Alice): " {
		t.Fatalf("expected 'Name (Alice): ', got '%v'\n", string(out.Bytes()))
	}
}

// TestForStringLabelDelimiter ensures that a default delimiter override is respected.
func TestWithDelimiter(t *testing.T) {
	var out bytes.Buffer
	var in bytes.Buffer
	in.Write([]byte("\n")) // [ENTER]

	_ = prompt.ForString("", "",
		prompt.WithInput(&in),
		prompt.WithOutput(&out),
		prompt.WithDelimiter("Δ"))
	if string(out.Bytes()) != "Δ" {
		t.Fatalf("expected output to be 'Δ', got '%v'\n", string(out.Bytes()))
	}
}

// TestForStringDefault ensures that the default is returned when enter is
// pressed on a string input.
func TestForStringDefault(t *testing.T) {
	var out bytes.Buffer
	var in bytes.Buffer
	in.Write([]byte("\n")) // [ENTER]

	// Empty default should return an empty value.
	s := prompt.ForString("", "",
		prompt.WithInput(&in),
		prompt.WithOutput(&out))

	if s != "" {
		t.Fatalf("expected '', got '%v'\n", s)
	}

	in.Reset()
	out.Reset()
	in.Write([]byte("\n")) // [ENTER]

	// Extant default should be returned
	s = prompt.ForString("", "default",
		prompt.WithInput(&in),
		prompt.WithOutput(&out))

	if s != "default" {
		t.Fatalf("expected 'default', got '%v'\n", s)
	}
}

// TestForStringRequired ensures that an error is generated if a value is not
// provided for a required prompt with no default.
func TestForStringRequired(t *testing.T) {
	var out bytes.Buffer
	var in bytes.Buffer
	in.Write([]byte("\n")) // [ENTER]

	_ = prompt.ForString("", "",
		prompt.WithInput(&in),
		prompt.WithOutput(&out),
		prompt.WithRequired(true),
		prompt.WithRetryLimit(1)) // makes the output buffer easier to confirm

	output := string(out.Bytes())
	expected := ": \nplease enter a value\n: "
	if output != expected {
		t.Fatalf("Unexpected prompt received for a required value.", expected, output)
	}
}

// TestForString ensures that string input is accepted.
func TestForString(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer
	in.Write([]byte("hunter2\n"))

	s := prompt.ForString("", "",
		prompt.WithInput(&in),
		prompt.WithOutput(&out))
	if s != "hunter2" {
		t.Fatalf("Expected 'hunter2' got '%v'", s)
	}
}

// TestForBoolLabel ensures that a prompt for a given boolean prompt prints
// the expected y/n prompt.
func TestForBoolLabel(t *testing.T) {
	var out bytes.Buffer
	var in bytes.Buffer
	in.Write([]byte("\n"))

	// Empty label, default false
	_ = prompt.ForBool("", false,
		prompt.WithInput(&in), prompt.WithOutput(&out))
	if string(out.Bytes()) != "(y/N): " {
		t.Fatalf("expected output to be '(y/N): ', got '%v'\n", string(out.Bytes()))
	}

	out.Reset()
	in.Reset()

	in.Write([]byte("\n"))

	// Empty label, default true
	_ = prompt.ForBool("", true,
		prompt.WithInput(&in), prompt.WithOutput(&out))
	if string(out.Bytes()) != "(Y/n): " {
		t.Fatalf("expected output to be '(Y/n): ', got '%v'\n", string(out.Bytes()))
	}

	out.Reset()
	in.Reset()

	in.Write([]byte("\n"))

	// Populated lablel default false
	_ = prompt.ForBool("Local", false,
		prompt.WithInput(&in), prompt.WithOutput(&out))
	if string(out.Bytes()) != "Local (y/N): " {
		t.Fatalf("expected 'Local (y/N): ', got '%v'\n", string(out.Bytes()))
	}
}

// TestForBoolDefault ensures that the default is returned when no user input is given.
func TestForBoolDefault(t *testing.T) {
	var out bytes.Buffer
	var in bytes.Buffer
	in.Write([]byte("\n"))

	b := prompt.ForBool("", false,
		prompt.WithInput(&in), prompt.WithOutput(&out))
	if b != false {
		t.Fatal("expected default of false to be returned when user accepts.")
	}

	out.Reset()
	in.Reset()

	in.Write([]byte("\n"))

	b = prompt.ForBool("", true,
		prompt.WithInput(&in), prompt.WithOutput(&out))
	if b != true {
		t.Fatal("expected default of true to be returned when user accepts.")
	}
}

// TestForBool ensures that a truthy value, when entered, is returned as a bool.
func TestForBool(t *testing.T) {
	var out bytes.Buffer
	var in bytes.Buffer

	cases := []struct {
		in  string
		out bool
	}{
		{"true", true},
		{"1", true},
		{"y", true},
		{"Y", true},
		{"yes", true},
		{"Yes", true},
		{"YES", true},
		{"false", false},
		{"0", false},
		{"n", false},
		{"N", false},
		{"no", false},
		{"No", false},
		{"NO", false},
	}

	for _, c := range cases {
		in.Reset()
		out.Reset()
		fmt.Fprintf(&in, "%v\n", c.in)

		// Note the default value is always the oposite of the input
		// to ensure it is flipped.
		b := prompt.ForBool("", !c.out,
			prompt.WithInput(&in), prompt.WithOutput(&out))
		if b != c.out {
			t.Fatalf("expected '%v' to be an acceptable %v.", c.in, c.out)
		}

	}
}
