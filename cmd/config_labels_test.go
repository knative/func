// +build linux

package cmd

import (
	"context"
	"github.com/Netflix/go-expect"
	"github.com/hinshun/vt10x"
	"github.com/spf13/cobra"
	fn "knative.dev/kn-plugin-func"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
)

type mockFunctionLoaderSaver struct {
	f fn.Function
}

func (m *mockFunctionLoaderSaver) Load(path string) (fn.Function, error) {
	return m.f, nil
}

func (m *mockFunctionLoaderSaver) Save(f fn.Function) error {
	m.f = f
	return nil
}

func assertLabelEq(t *testing.T, actual fn.Labels, want fn.Labels) {
	t.Helper()
	if !reflect.DeepEqual(actual, want) {
		t.Errorf("labels =  %v, want %v", actual, want)
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
			_,_ = c.ExpectEOF()
		}()
		go func() {
			defer wg.Done()
			time.Sleep(time.Millisecond*50)
			for _, s := range input {
				_, _ = c.Send(s)
				time.Sleep(time.Millisecond*50)
			}
		}()

		a := []string{subcmd}
		cmd.SetArgs(a)

		func () {
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
	arrowUp = "\033[A"
	arrowDown = "\033[B"
	enter = "\r"
)

func TestNewConfigLabelsCmd(t *testing.T) {

	var loaderSaver mockFunctionLoaderSaver
	labels := &loaderSaver.f.Labels

	cmd := NewConfigLabelsCmd(&loaderSaver)

	run := createRunFunc(cmd, t)


	p := func (k,v string) fn.Label {
		return fn.Label{Key: &k, Value: &v}
	}

	assertLabel := func (ps fn.Labels) {
		t.Helper()
		assertLabelEq(t, *labels, ps)
	}

	run("add", enter, "a", enter, "b", enter)
	assertLabel(fn.Labels{p("a","b")})

	run("add", enter, enter, "c", enter, "d", enter)
	assertLabel(fn.Labels{p("a","b"), p("c", "d")})

	run("add", arrowUp, arrowUp, enter, enter, "e", enter, "f", enter)
	assertLabel( fn.Labels{p("e","f"), p("a","b"), p("c", "d")})

	run("remove", arrowDown, enter)
	assertLabel(fn.Labels{p("e","f"), p("c", "d")})
}
