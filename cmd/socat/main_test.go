package main

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootCmd(t *testing.T) {

	/* Begin prepare TCP server and the file begin */
	addr := startTCPEcho(t)

	const fileContent = "file-content"
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "a.txt")
	err := os.WriteFile(testFile, []byte(fileContent), 0644)
	if err != nil {
		t.Fatal(err)
	}
	/* End prepare TCP server and the file begin */

	type args struct {
		args          []string
		inputString   string
		outMatcher    func(string) bool
		errOutMatcher func(string) bool
		wantErr       bool
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "stdio<->tcp",
			args: args{
				args:          []string{"-", "TCP:" + addr},
				inputString:   "tcp-echo",
				outMatcher:    func(s string) bool { return s == "tcp-echo" },
				errOutMatcher: func(s string) bool { return true },
			},
		},
		{
			name: "tcp-no-such-host",
			args: args{
				args:          []string{"-", "TCP:does.not.exist:10000"},
				inputString:   "tcp-echo",
				outMatcher:    func(s string) bool { return true },
				errOutMatcher: func(s string) bool { return strings.Contains(s, "not resolve") },
				wantErr:       true,
			},
		},
		{
			name: "file->stdio",
			args: args{
				args:          []string{"-u", "OPEN:" + testFile, "-"},
				inputString:   "",
				outMatcher:    func(s string) bool { return s == fileContent },
				errOutMatcher: func(s string) bool { return true },
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer

			cmd := NewRootCmd()
			cmd.SetIn(io.NopCloser(strings.NewReader(tt.args.inputString)))
			cmd.SetOut(noopWriterCloser{&out})
			cmd.SetErr(noopWriterCloser{&errOut})
			cmd.SetArgs(tt.args.args)

			err = cmd.Execute()
			if err != nil && !tt.args.wantErr {
				t.Error(err)
				t.Logf("errOut: %q", errOut.String())
			}

			if err == nil && tt.args.wantErr {
				t.Error("expected error but got nil")
			}

			if !tt.args.outMatcher(out.String()) {
				t.Error("bad output")
			}
			if !tt.args.errOutMatcher(errOut.String()) {
				t.Error("bat error output")
			}
		})
	}
}

type noopWriterCloser struct {
	io.Writer
}

func (n noopWriterCloser) Close() error {
	return nil
}

func startTCPEcho(t *testing.T) (addr string) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	addr = l.Addr().String()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				panic(err)
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, err = io.Copy(conn, conn)
				if err != nil {
					panic(err)
				}
			}(conn)
		}
	}()
	t.Cleanup(func() {
		l.Close()
	})
	return addr
}
