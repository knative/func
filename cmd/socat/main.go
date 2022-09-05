package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func main() {
	cmd := newRootCmd()
	err := cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var uniDir bool
	cmd := cobra.Command{
		Use:   "socat <address> <address>",
		Short: "Minimalistic socat.",
		Long: `Minimalistic socat.
Implements only TCP, OPEN and stdio ("-") addresses with no options.
Only supported flag is -u.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			right, err := createConnection(args[0])
			if err != nil {
				return err
			}
			left, err := createConnection(args[1])
			if err != nil {
				return err
			}
			return connect(right, left, uniDir)
		},
	}

	cmd.Flags().BoolVarP(&uniDir, "", "u", false, "unidirectional mode (left to right)")

	return &cmd
}

func createConnection(address string) (connection, error) {
	if address == "-" {
		return stdio, nil
	}
	parts := strings.SplitN(address, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("cannot parse address: %q", address)
	}
	typ := strings.ToLower(parts[0])
	parts = strings.Split(parts[1], ",")
	if len(parts) > 1 {
		_, _ = fmt.Fprintf(os.Stderr, "flags ignored: %q\n", parts[1])
	}
	addr := parts[0]
	switch typ {
	case "tcp", "tcp4", "tcp6":
		var laddr net.TCPAddr
		raddr, err := net.ResolveTCPAddr(typ, addr)
		if err != nil {
			return nil, fmt.Errorf("name does not resolve: %w", err)
		}
		return net.DialTCP(typ, &laddr, raddr)
	case "open":
		return os.OpenFile(addr, os.O_RDWR, 0644)
	}
	return nil, fmt.Errorf("unsupported address: %q", address)
}

func connect(a, b connection, uniDir bool) error {
	errChan := make(chan error, 1)

	if !uniDir {
		go func() {
			_, err := io.Copy(a, b)
			_ = tryCloseWriter(a)
			errChan <- err
		}()
	} else {
		errChan <- nil
	}

	_, err := io.Copy(b, a)
	_ = tryCloseWriter(b)
	if err != nil {
		return err
	}

	return <-errChan
}

type connection interface {
	io.Reader
	io.Writer
	io.Closer
}

type writeCloser interface {
	CloseWrite() error
}

type rwc struct {
	r io.ReadCloser
	w io.WriteCloser
}

func (r rwc) Read(p []byte) (n int, err error) {
	return r.r.Read(p)
}

func (r rwc) Write(p []byte) (n int, err error) {
	return r.w.Write(p)
}

func (r rwc) Close() error {
	err := r.w.Close()
	if err != nil {
		return err
	}
	return r.r.Close()
}

func (r rwc) CloseWrite() error {
	return r.w.Close()
}

func tryCloseWriter(c connection) error {
	if wc, ok := c.(writeCloser); ok {
		return wc.CloseWrite()
	}
	return nil
}

var stdio = rwc{
	r: os.Stdin,
	w: os.Stdout,
}
