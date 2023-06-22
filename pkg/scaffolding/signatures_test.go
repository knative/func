package scaffolding

import (
	"fmt"
	"testing"
)

// TestSignatures ensures that the enum of signatures is properly formed.
// - The correct signature enum is returned from ToSignature
// - The string method is correctly indexed
// - "http" is the default for invocation hint
func TestSignatures(t *testing.T) {

	// This is a brute-force implementation which simply runs the logic in
	// reverse.  Basically just checking ones work when the enum is modified.

	tests := []struct {
		instanced    bool      // signatures are brodly classified into instanced or static
		invocation   string    // the invocation hint (default is "http")
		expectedEnum Signature // the expected enum
		expectedName string    // the expected string
	}{
		{true, "", InstancedHTTP, "instanced-http"},
		{true, "http", InstancedHTTP, "instanced-http"},
		{true, "cloudevent", InstancedCloudevents, "instanced-cloudevents"},
		{false, "", StaticHTTP, "static-http"},
		{false, "http", StaticHTTP, "static-http"},
		{false, "cloudevent", StaticCloudevents, "static-cloudevents"},
		{true, "invalid", UnknownSignature, "unknown"},
		{false, "invalid", UnknownSignature, "unknown"},
	}

	testName := func(instanced bool, invocation string) string {
		instancedString := "instanced"
		if !instanced {
			instancedString = "static"
		}
		invocationString := "default"
		if invocation != "" {
			invocationString = invocation
		}
		return fmt.Sprintf("%v-%v", instancedString, invocationString)
	}

	for _, test := range tests {
		t.Run(testName(test.instanced, test.invocation), func(t *testing.T) {

			signature := toSignature(test.instanced, test.invocation)

			if signature != test.expectedEnum {
				t.Fatal("enum incorrectly mapped.")
			}
			if signature.String() != test.expectedName {
				t.Fatal("string representation incorrectly mapped")
			}

		})
	}
}
