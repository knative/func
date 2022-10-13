//go:build linux
// +build linux

package cmd

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/hinshun/vt10x"

	"knative.dev/func/docker"
)

func Test_newPromptForCredentials(t *testing.T) {
	expectedCreds := docker.Credentials{
		Username: "testuser",
		Password: "testpwd",
	}

	console, _, err := vt10x.NewVT10XConsole()
	if err != nil {
		t.Fatal(err)
	}
	defer console.Close()

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
			credPrompt := newPromptForCredentials(tt.in, tt.out, tt.errOut)
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
