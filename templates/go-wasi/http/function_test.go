package main

import "testing"

// TestGreet verifies the pure greet helper in isolation.
// The WASI handler itself (handle) cannot be called in a standard Go test
// without a WASM runtime — but the pure logic can be tested directly.
func TestGreet(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/", "Hello from WASI! Path: /\n"},
		{"/greet", "Hello from WASI! Path: /greet\n"},
		{"/greet?name=world", "Hello from WASI! Path: /greet?name=world\n"},
	}
	for _, tt := range tests {
		got := greet(tt.path)
		if got != tt.want {
			t.Errorf("greet(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
