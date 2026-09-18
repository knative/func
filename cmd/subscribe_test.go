package cmd

import (
	"reflect"
	"testing"

	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

func TestSubscribeWithAll(t *testing.T) {
	root := FromTempDirectory(t)

	_, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewSubscribeCmd()
	cmd.SetArgs([]string{"--source", "my-broker", "--filter", "foo=go"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Now load the function and ensure that the subscription is set correctly.
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if f.Deploy.Subscriptions == nil {
		t.Fatal("Expected subscription to be present ")
	}
	if f.Deploy.Subscriptions[0].Source != "my-broker" {
		t.Fatalf("Expected subscription for broker to be 'my-broker', but got '%v'", f.Deploy.Subscriptions[0].Source)
	}

	if f.Deploy.Subscriptions[0].Filters["foo"] != "go" {
		t.Fatalf("Expected subscription filter for 'foo' to be 'go', but got '%v'", f.Deploy.Subscriptions[0].Filters["foo"])
	}
}

func TestSubscribeWithMultiple(t *testing.T) {
	root := FromTempDirectory(t)

	_, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewSubscribeCmd()
	cmd.SetArgs([]string{"--source", "my-broker", "--filter", "foo=go"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Now load the function and ensure that the subscription is set correctly.
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if f.Deploy.Subscriptions == nil {
		t.Fatal("Expected subscription to be present ")
	}
	if f.Deploy.Subscriptions[0].Source != "my-broker" {
		t.Fatalf("Expected subscription for broker to be 'my-broker', but got '%v'", f.Deploy.Subscriptions[0].Source)
	}

	if f.Deploy.Subscriptions[0].Filters["foo"] != "go" {
		t.Fatalf("Expected subscription filter for 'foo' to be 'go', but got '%v'", f.Deploy.Subscriptions[0].Filters["foo"])
	}

	cmd = NewSubscribeCmd()
	cmd.SetArgs([]string{"--source", "my-broker", "--filter", "bar=foo"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Now load the function and ensure that the subscription is set correctly.
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if f.Deploy.Subscriptions == nil {
		t.Fatal("Expected subscription to be present ")
	}
	if f.Deploy.Subscriptions[0].Source != "my-broker" {
		t.Fatalf("Expected subscription for broker to be 'my-broker', but got '%v'", f.Deploy.Subscriptions[0].Source)
	}

	if f.Deploy.Subscriptions[0].Filters["foo"] != "go" {
		t.Fatalf("Expected subscription filter for 'foo' to be 'go', but got '%v'", f.Deploy.Subscriptions[0].Filters["foo"])
	}
	if f.Deploy.Subscriptions[0].Filters["bar"] != "foo" {
		t.Fatalf("Expected subscription filter for 'bar' to be 'foo', but got '%v'", f.Deploy.Subscriptions[0].Filters["foo"])
	}

}

func TestSubscribeWithMultipleBrokersAndOverride(t *testing.T) {
	root := FromTempDirectory(t)

	_, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewSubscribeCmd()
	cmd.SetArgs([]string{"--source", "my-broker", "--filter", "foo=go"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Now load the function and ensure that the subscription is set correctly.
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if f.Deploy.Subscriptions == nil {
		t.Fatal("Expected subscription to be present ")
	}
	if f.Deploy.Subscriptions[0].Source != "my-broker" {
		t.Fatalf("Expected subscription for broker to be 'my-broker', but got '%v'", f.Deploy.Subscriptions[0].Source)
	}

	if f.Deploy.Subscriptions[0].Filters["foo"] != "go" {
		t.Fatalf("Expected subscription filter for 'foo' to be 'go', but got '%v'", f.Deploy.Subscriptions[0].Filters["foo"])
	}

	cmd = NewSubscribeCmd()
	cmd.SetArgs([]string{"--filter", "bar=foo"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Now load the function and ensure that the subscription is set correctly.
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if f.Deploy.Subscriptions == nil {
		t.Fatal("Expected subscription to be present ")
	}
	if f.Deploy.Subscriptions[1].Source != "default" {
		t.Fatalf("Expected subscription for broker to be 'default', but got '%v'", f.Deploy.Subscriptions[0].Source)
	}

	if f.Deploy.Subscriptions[1].Filters["bar"] != "foo" {
		t.Fatalf("Expected subscription filter for 'bar' to be 'foo', but got '%v'", f.Deploy.Subscriptions[0].Filters["foo"])
	}

	cmd = NewSubscribeCmd()
	cmd.SetArgs([]string{"--source", "my-broker", "--filter", "foo=golang"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Now load the function and ensure that the subscription is set correctly.
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if f.Deploy.Subscriptions == nil {
		t.Fatal("Expected subscription to be present ")
	}
	if f.Deploy.Subscriptions[0].Source != "my-broker" {
		t.Fatalf("Expected subscription for broker to be 'my-broker', but got '%v'", f.Deploy.Subscriptions[0].Source)
	}

	if f.Deploy.Subscriptions[0].Filters["foo"] != "golang" {
		t.Fatalf("Expected subscription filter for 'foo' to be 'golang', but got '%v'", f.Deploy.Subscriptions[0].Filters["foo"])
	}
}

func TestSubscribeWithNoExplicitSourceAll(t *testing.T) {
	root := FromTempDirectory(t)

	_, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewSubscribeCmd()
	cmd.SetArgs([]string{"--filter", "foo=go"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Now load the function and ensure that the subscription is set correctly.
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if f.Deploy.Subscriptions == nil {
		t.Fatal("Expected subscription to be present ")
	}
	if f.Deploy.Subscriptions[0].Source != "default" {
		t.Fatalf("Expected subscription for broker to be 'default', but got '%v'", f.Deploy.Subscriptions[0].Source)
	}

	if f.Deploy.Subscriptions[0].Filters["foo"] != "go" {
		t.Fatalf("Expected subscription filter for 'foo' to be 'go', but got '%v'", f.Deploy.Subscriptions[0].Filters["foo"])
	}
}

func TestSubscribeWithDuplicated(t *testing.T) {
	root := FromTempDirectory(t)

	_, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewSubscribeCmd()
	cmd.SetArgs([]string{"--source", "my-broker", "--filter", "foo=go"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Now load the function and ensure that the subscription is set correctly.
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if f.Deploy.Subscriptions == nil {
		t.Fatal("Expected subscription to be present ")
	}
	if f.Deploy.Subscriptions[0].Source != "my-broker" {
		t.Fatalf("Expected subscription for broker to be 'my-broker', but got '%v'", f.Deploy.Subscriptions[0].Source)
	}

	if f.Deploy.Subscriptions[0].Filters["foo"] != "go" {
		t.Fatalf("Expected subscription filter for 'foo' to be 'go', but got '%v'", f.Deploy.Subscriptions[0].Filters["foo"])
	}

	// call it again with same
	cmd = NewSubscribeCmd()
	cmd.SetArgs([]string{"--source", "my-broker", "--filter", "foo=go"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	// Now load the function and ensure that the subscription is set correctly.
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(f.Deploy.Subscriptions) > 1 {
		t.Fatal("Expected only one subscription to be present ")
	}

	// call it again and override
	cmd = NewSubscribeCmd()
	cmd.SetArgs([]string{"--source", "my-broker", "--filter", "foo=gogo"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Now load the function and ensure that the subscription is set correctly.
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(f.Deploy.Subscriptions) > 1 {
		t.Fatal("Expected only one subscription to be present ")
	}
	if f.Deploy.Subscriptions[0].Filters["foo"] != "gogo" {
		t.Fatalf("Expected subscription filter for 'foo' to be 'gogo', but got '%v'", f.Deploy.Subscriptions[0].Filters["foo"])
	}

}

// TestSubscribeRejectsMalformedFilter ensures that a --filter value which is
// not in key=value form fails the command rather than being silently dropped,
// and that nothing is written to func.yaml.
func TestSubscribeRejectsMalformedFilter(t *testing.T) {
	root := FromTempDirectory(t)

	_, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewSubscribeCmd()
	cmd.SetArgs([]string{"--source", "my-broker", "--filter", "badfilter"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Expected an error for a malformed filter, but got nil")
	}

	// The function on disk must be left untouched.
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(f.Deploy.Subscriptions) != 0 {
		t.Fatalf("Expected no subscriptions to be written, but got '%v'", f.Deploy.Subscriptions)
	}
}

// TestSubscribeAllowsFilterValueContainingEquals ensures a filter value which
// itself contains "=" is preserved in full rather than discarded.
func TestSubscribeAllowsFilterValueContainingEquals(t *testing.T) {
	root := FromTempDirectory(t)

	_, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewSubscribeCmd()
	cmd.SetArgs([]string{"--source", "my-broker", "--filter", "foo=bar=baz"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if f.Deploy.Subscriptions == nil {
		t.Fatal("Expected subscription to be present ")
	}
	if f.Deploy.Subscriptions[0].Filters["foo"] != "bar=baz" {
		t.Fatalf("Expected subscription filter for 'foo' to be 'bar=baz', but got '%v'", f.Deploy.Subscriptions[0].Filters["foo"])
	}
}

// TestExtractFilterMap covers filter parsing directly, including values which
// themselves contain "=" and the malformed forms which must be rejected.
func TestExtractFilterMap(t *testing.T) {
	tests := []struct {
		name    string
		filters []string
		want    map[string]string
		wantErr bool
	}{
		{
			name:    "single valid filter",
			filters: []string{"type=com.example"},
			want:    map[string]string{"type": "com.example"},
		},
		{
			name:    "multiple valid filters",
			filters: []string{"type=com.example", "extension=my-value"},
			want:    map[string]string{"type": "com.example", "extension": "my-value"},
		},
		{
			name:    "value containing equals is preserved",
			filters: []string{"foo=bar=baz"},
			want:    map[string]string{"foo": "bar=baz"},
		},
		{
			name:    "empty value is allowed",
			filters: []string{"key="},
			want:    map[string]string{"key": ""},
		},
		{
			name:    "no filters yields empty map",
			filters: []string{},
			want:    map[string]string{},
		},
		{
			name:    "missing equals is rejected",
			filters: []string{"badfilter"},
			wantErr: true,
		},
		{
			name:    "empty key is rejected",
			filters: []string{"=value"},
			wantErr: true,
		},
		{
			name:    "one malformed filter rejects the whole set",
			filters: []string{"type=com.example", "badfilter"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractFilterMap(tt.filters)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Expected an error, but got nil (result '%v')", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Expected '%v', but got '%v'", tt.want, got)
			}
		})
	}
}
