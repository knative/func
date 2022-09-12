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
	cmd := NewRootCmd()
	err := cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func NewRootCmd() *cobra.Command {
	var uniDir bool
	cmd := cobra.Command{
		Use:   "socat <address> <address>",
		Short: "Minimalistic socat.",
		Long: `Minimalistic socat.
Implements only TCP, OPEN and stdio ("-") addresses with no options.
Only supported flag is -u.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			left, err := createConnection(args[0])
			if err != nil {
				return err
			}
			defer left.Close()
			right, err := createConnection(args[1])
			if err != nil {
				return err
			}
			defer right.Close()
			return connect(left, right, uniDir)
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
		_, _ = fmt.Fprintf(os.Stderr, "ignored options: %q\n", parts[1])
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
	default:
		return nil, fmt.Errorf("unsupported address: %q", address)
	}
}

func connect(left, right connection, uniDir bool) error {
	errChan := make(chan error, 1)

	if !uniDir {
		go func() {
			_, err := io.Copy(left, right)
			tryCloseWriteSide(left)
			errChan <- err
		}()
	} else {
		errChan <- nil
	}

	_, err := io.Copy(right, left)
	tryCloseWriteSide(right)
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
	io.ReadCloser
	io.WriteCloser
}

func (r rwc) Close() error {
	err := r.WriteCloser.Close()
	if err != nil {
		return err
	}
	return r.ReadCloser.Close()
}

func (r rwc) CloseWrite() error {
	return r.WriteCloser.Close()
}

func tryCloseWriteSide(c connection) {
	if wc, ok := c.(writeCloser); ok {
		err := wc.CloseWrite()
		if err != nil {
			fmt.Fprintf(os.Stderr, "waring: cannot close write side: %+v\n", err)
		}
	}
}

var stdio = rwc{
	ReadCloser:  os.Stdin,
	WriteCloser: os.Stdout,
}
