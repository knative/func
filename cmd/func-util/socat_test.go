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

	/* Begin prepare TCP server and the files */
	addr := startTCPEcho(t)

	const testData = "file-content\n"
	tmpDir := t.TempDir()
	inputFile := filepath.Join(tmpDir, "a.txt")
	err := os.WriteFile(inputFile, []byte(testData), 0644)
	if err != nil {
		t.Fatal(err)
	}

	outputFile := filepath.Join(tmpDir, "b.txt")
	err = os.WriteFile(outputFile, []byte{}, 0644)
	if err != nil {
		t.Fatal(err)
	}
	/* End prepare TCP server and the files */

	type matcher = func(string) bool
	contains := func(pattern string) func(string) bool {
		return func(s string) bool { return strings.Contains(s, pattern) }
	}
	equalsTo := func(pattern string) func(string) bool {
		return func(s string) bool { return s == pattern }
	}

	type args struct {
		args           []string
		inputString    string
		outMatcher     matcher
		errOutMatcher  matcher
		outFileMatcher matcher
		wantErr        bool
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "stdio<->tcp",
			args: args{
				args:        []string{"-", "TCP:" + addr},
				inputString: testData,
				outMatcher:  equalsTo(testData),
			},
		},
		{
			name: "tcp<->stdio",
			args: args{
				args:        []string{"TCP:" + addr, "-"},
				inputString: testData,
				outMatcher:  equalsTo(testData),
			},
		},
		{
			name: "tcp-no-such-host",
			args: args{
				args:          []string{"-", "TCP:does.not.exist:10000"},
				inputString:   "tcp-echo",
				errOutMatcher: contains("not resolve"),
				wantErr:       true,
			},
		},
		{
			name: "file->stdio",
			args: args{
				args:        []string{"-u", "OPEN:" + inputFile, "-"},
				inputString: "",
				outMatcher:  equalsTo(testData),
			},
		},
		{
			name: "stdio->file",
			args: args{
				args:           []string{"-u", "-", "OPEN:" + outputFile},
				inputString:    testData,
				outFileMatcher: equalsTo(testData),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer

			stdout := &testWriter{Writer: &out}
			stderr := &testWriter{Writer: &errOut}
			cmd := newSocatCmd()
			cmd.SetIn(io.NopCloser(strings.NewReader(tt.args.inputString)))
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)
			cmd.SetArgs(tt.args.args)

			err = cmd.Execute()
			if err != nil && !tt.args.wantErr {
				t.Error(err)
				t.Logf("errOut: %q", errOut.String())
			}

			if err == nil && tt.args.wantErr {
				t.Error("expected error but got nil")
			}

			if tt.args.outMatcher != nil && !tt.args.outMatcher(out.String()) {
				t.Error("bad standard output")
			}
			if tt.args.errOutMatcher != nil && !tt.args.errOutMatcher(errOut.String()) {
				t.Error("bad standard error output")
			}
			if tt.args.outFileMatcher != nil {
				bs, e := os.ReadFile(outputFile)
				if e != nil {
					t.Fatal(e)
				}
				if !tt.args.outFileMatcher(string(bs)) {
					t.Error("bad content of the output file")
				}
			}
		})
	}
}

type testWriter struct {
	io.Writer
}

func (n *testWriter) Close() error {
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

func TestNewRootCmdWithPipe(t *testing.T) {
	addr := startTCPEcho(t)

	r, stdOut, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	stdIn, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	var data = []byte("testing data")

	go func() {
		var err error
		_, err = w.Write(data)
		if err != nil {
			t.Error(err)
		}
		err = w.Close()
		if err != nil {
			t.Error(err)
		}
	}()

	go func() {
		var err error
		var errBuff bytes.Buffer
		cmd := newSocatCmd()
		cmd.SetIn(stdIn)
		cmd.SetOut(stdOut)
		cmd.SetErr(&errBuff)
		cmd.SetArgs([]string{"-dd", "-", "TCP:" + addr})

		err = cmd.Execute()
		if err != nil {
			t.Error(err)
		}

	}()

	bs, e := io.ReadAll(r)
	if e != nil {
		t.Error(e)
	}
	t.Log(string(data))
	if !bytes.Equal(data, bs) {
		t.Errorf("bad data: %q", string(bs))
	}
}
