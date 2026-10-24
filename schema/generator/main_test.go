package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain runs the tests from the repository root, which is the working
// directory the generator is run from: it reads the docstrings under
// ./pkg/functions/.
func TestMain(m *testing.M) {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Chdir(root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// repoRoot walks up from the working directory to the module root.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod found above %s", dir)
		}
		dir = parent
	}
}

// TestTargetMinimumIsPreserved is the regression test for the reported bug.
//
// The generator published "minimum": 0 for KPAScaleOptions.target, because
// reflection truncated the fractional value, while the runtime validation
// requires target >= 0.01. The schema was therefore looser than the validation
// it documents: target: 0.001 passed the schema and was rejected at deploy.
// The integral bounds of the neighbouring properties are asserted alongside it,
// so the correction cannot be mistaken for a general distortion of the
// reflected numbers.
//
// It also asserts that the exclusiveMinimum workaround that stood in for the
// truncated minimum is gone: under the draft-04 declared by this document, a
// boolean exclusiveMinimum would require target > 0.01 and so contradict
// validateKPAScale, which accepts exactly 0.01.
func TestTargetMinimumIsPreserved(t *testing.T) {
	schema, err := funcYamlSchema()
	if err != nil {
		t.Fatalf("cannot generate schema: %s", err)
	}

	var doc struct {
		Definitions map[string]struct {
			Properties map[string]map[string]json.RawMessage `json:"properties"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(schema, &doc); err != nil {
		t.Fatalf("cannot parse schema: %s", err)
	}

	// The verbatim JSON token is compared rather than a decoded number, so an
	// integral bound cannot pass as the fractional one.
	tests := []struct {
		definition string
		property   string
		keyword    string
		want       string
	}{
		// the reported bug
		{"KPAScaleOptions", "target", "minimum", "0.01"},
		// the neighbouring integral bounds must be unaffected
		{"KPAScaleOptions", "utilization", "minimum", "1"},
		{"KPAScaleOptions", "utilization", "maximum", "100"},
	}
	for _, test := range tests {
		got := string(doc.Definitions[test.definition].Properties[test.property][test.keyword])
		if got != test.want {
			t.Errorf("%s.%s %s = %s, want %s", test.definition, test.property, test.keyword, got, test.want)
		}
	}

	// This document declares draft-04, where a boolean exclusiveMinimum makes
	// minimum exclusive: target > 0.01 would be required, while
	// validateKPAScale accepts target >= 0.01. The keyword was only ever
	// present as a workaround for the truncation restored above, so it must be
	// gone now that minimum is emitted as the fractional value.
	if got := string(doc.Definitions["KPAScaleOptions"].Properties["target"]["exclusiveMinimum"]); got == "true" {
		t.Errorf("KPAScaleOptions.target exclusiveMinimum = %s, want absent: it requires target > 0.01, but validateKPAScale accepts target >= 0.01", got)
	}
}
