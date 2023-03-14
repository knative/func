package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Netflix/go-expect"
	"github.com/hinshun/vt10x"
	"github.com/spf13/cobra"
	fn "knative.dev/func/pkg/functions"
)

// TestLabels_DefaultEmpty ensures that the default is a list which responds
// correctly when no labels are specified, in both default and json output
func TestLabels_DefaultEmpty(t *testing.T) {
	root := fromTempDirectory(t)
	if err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// Empty list, default output (human legible)
	cmd := NewLabelsCmd(NewTestClient())
	cmd.SetArgs([]string{})
	buff := bytes.Buffer{}
	cmd.SetOut(&buff)
	cmd.SetErr(&buff)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimSpace(buff.String())
	if out != "No labels" {
		t.Fatalf("Unexpected result from an empty labels list:\n%v\n", out)
	}

	// Empty list, json output
	cmd = NewLabelsCmd(NewTestClient())
	cmd.SetArgs([]string{"-o=json"})
	buff = bytes.Buffer{}
	cmd.SetOut(&buff)
	cmd.SetErr(&buff)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var data []fn.Label
	bb := buff.Bytes()
	if err := json.Unmarshal(bb, &data); err != nil {
		t.Fatal(err) // fail if not proper json
	}
	if !reflect.DeepEqual(data, []fn.Label{}) {
		t.Fatalf("unexpected data from an empty function: %s\n", bb)
	}
}

// TestLabels_DefaultPopulated ensures that labels on a function are
// listed in both default and json formats.
func TestLabels_DefaultPopulated(t *testing.T) {
	root := fromTempDirectory(t)

	key := "key"
	value := "value"
	labels := []fn.Label{{Key: &key, Value: &value}} // TODO: pointers unnecessary
	if err := fn.New().Init(fn.Function{Runtime: "go", Root: root, Deploy: fn.DeploySpec{Labels: labels}}); err != nil {
		t.Fatal(err)
	}

	// Populated list, default formatting
	cmd := NewLabelsCmd(NewTestClient())
	cmd.SetArgs([]string{})
	buff := bytes.Buffer{}
	cmd.SetOut(&buff)
	cmd.SetErr(&buff)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimSpace(buff.String())
	if out != "Labels:\n  Label with key \"key\" and value \"value\"" {
		t.Fatalf("Unexpected labels list:\n%v\n", out)
	}

	// Populated list, json output
	cmd = NewLabelsCmd(NewTestClient())
	cmd.SetArgs([]string{"-o=json"})
	buff = bytes.Buffer{}
	cmd.SetOut(&buff)
	cmd.SetErr(&buff)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var data []fn.Label
	bb := buff.Bytes()
	if err := json.Unmarshal(bb, &data); err != nil {
		t.Fatal(err) // fail if not proper json
	}
	if !reflect.DeepEqual(data, labels) {
		t.Fatalf("expected output: %s got: %s\n", labels, data)
	}
}

// TestLabels_Add ensures adding labels works as expected.
func TestLabels_Add(t *testing.T) {
	root := fromTempDirectory(t)

	if err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// Add a simple value
	cmd := NewLabelsCmd(NewTestClient())
	cmd.SetArgs([]string{"add", "--key=key", "--value=value"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	key := "key" // TODO: pointers unnecessary
	value := "value"
	labels := []fn.Label{{Key: &key, Value: &value}}
	if !reflect.DeepEqual(f.Deploy.Labels, labels) {
		t.Fatalf("unexpected labels: %v\n", f.Deploy.Labels)
	}

	// Add an environment variable
	cmd = NewLabelsCmd(NewTestClient())
	cmd.SetArgs([]string{"add", "--key=key", "--value={{ env:MYENV }}"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	key2 := "key"
	value2 := "{{ env:MYENV }}"
	labels = []fn.Label{
		{Key: &key, Value: &value},
		{Key: &key2, Value: &value2},
	}
	if !reflect.DeepEqual(f.Deploy.Labels, labels) {
		t.Fatalf("expected labels: \n%v\nGot labels: \n%v\n", labels, f.Deploy.Labels)
	}

	// TODO: check errors when label or key invalid
}

// TestLabels_Remove ensures that removing works as expected
func TestLabels_Remove(t *testing.T) {
}

// TestLabels_Interactive ensures the expected interactive flow succeeds.
func TestLabels_Interactive(t *testing.T) {
	root := fromTempDirectory(t)
	if err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	cmd := NewLabelsCmd(NewTestClient())
	cmd.SetArgs([]string{})

	run := createRunFunc(cmd, t)

	p := func(k, v string) fn.Label { // TODO: pointers unnecessary
		return fn.Label{Key: &k, Value: &v}
	}

	assert := func(ll []fn.Label) {
		assertLabels(t, root, ll)
	}

	run("add", enter, "a", enter, "b", enter)
	assert([]fn.Label{p("a", "b")})

	run("add", enter, enter, "c", enter, "d", enter)
	assert([]fn.Label{p("a", "b"), p("c", "d")})

	run("add", arrowUp, arrowUp, enter, enter, "e", enter, "f", enter)
	assert([]fn.Label{p("e", "f"), p("a", "b"), p("c", "d")})

	run("remove", arrowDown, enter)
	assert([]fn.Label{p("e", "f"), p("c", "d")})
}

func assertLabels(t *testing.T, root string, expected []fn.Label) {
	t.Helper()
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.Deploy.Labels, expected) {
		t.Errorf("expected labels: %v, got: %v", expected, f.Deploy.Labels)
	}
}

func createRunFunc(cmd *cobra.Command, t *testing.T) func(subcmd string, input ...string) {
	return func(subcmd string, input ...string) {

		ctx := context.Background()
		c, _, err := vt10x.NewVT10XConsole()
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			//defer wg.Done()
			_, _ = c.ExpectEOF()
		}()
		go func() {
			defer wg.Done()
			time.Sleep(time.Millisecond * 50)
			for _, s := range input {
				_, _ = c.Send(s)
				time.Sleep(time.Millisecond * 50)
			}
		}()

		a := []string{subcmd}
		cmd.SetArgs(a)

		func() {
			defer withMockedStdio(t, c)()
			err = cmd.ExecuteContext(ctx)
			wg.Wait()
		}()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func withMockedStdio(t *testing.T, c *expect.Console) func() {
	t.Helper()

	oldIn := os.Stdin
	oldOut := os.Stdout
	oldErr := os.Stderr

	os.Stdin = c.Tty()
	os.Stdout = c.Tty()
	os.Stderr = c.Tty()

	return func() {
		os.Stdin = oldIn
		os.Stdout = oldOut
		os.Stderr = oldErr
	}
}

const (
	arrowUp   = "\033[A"
	arrowDown = "\033[B"
	enter     = "\r"
)
