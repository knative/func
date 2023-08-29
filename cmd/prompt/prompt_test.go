//go:build linux
// +build linux

package prompt

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Netflix/go-expect"
	"github.com/creack/pty"
	"github.com/hinshun/vt10x"

	"knative.dev/func/pkg/docker"
)

const (
	enter = "\r"
)

func Test_NewPromptForCredentials(t *testing.T) {
	expectedCreds := docker.Credentials{
		Username: "testuser",
		Password: "testpwd",
	}

	ptm, pts, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	term := vt10x.New(vt10x.WithWriter(pts))
	console, err := expect.NewConsole(expect.WithStdin(ptm), expect.WithStdout(term), expect.WithCloser(ptm, pts))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { console.Close() })

	go func() {
		_, _ = console.ExpectEOF()
	}()

	go func() {
		chars := expectedCreds.Username + enter + expectedCreds.Password + enter
		for _, ch := range chars {
			time.Sleep(time.Millisecond * 100)
			_, _ = console.Send(string(ch))
		}
	}()

	tests := []struct {
		name   string
		in     io.Reader
		out    io.Writer
		errOut io.Writer
	}{
		{
			name:   "with non-tty",
			in:     strings.NewReader(expectedCreds.Username + "\r\n" + expectedCreds.Password + "\r\n"),
			out:    io.Discard,
			errOut: io.Discard,
		},
		{
			name:   "with tty",
			in:     console.Tty(),
			out:    console.Tty(),
			errOut: console.Tty(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credPrompt := NewPromptForCredentials(tt.in, tt.out, tt.errOut)
			cred, err := credPrompt("example.com")
			if err != nil {
				t.Fatal(err)
			}
			if cred != expectedCreds {
				t.Errorf("bad credentials: %+v", cred)
			}
		})
	}
}
