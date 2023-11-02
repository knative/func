package cmd

import (
	"testing"

	fn "knative.dev/func/pkg/functions"
)

func TestSubscribeWithAll(t *testing.T) {
	root := fromTempDirectory(t)

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

func TestSubscribeWithNoExplicitSourceAll(t *testing.T) {
	root := fromTempDirectory(t)

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
