package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newSocatCmd() *cobra.Command {
	var (
		uniDir bool
		dbg    string
	)
	cmd := cobra.Command{
		Use:   "socat [-u] <address> <address>",
		Short: "Minimalistic socat.",
		Long: `Minimalistic socat.
Implements only TCP, OPEN and stdio ("-") addresses with no options.
Only supported flag is -u. Exits half a second after one direction
ends.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			stdio := rwc{
				ReadCloser:  cmd.InOrStdin().(io.ReadCloser),
				WriteCloser: cmd.OutOrStdout().(io.WriteCloser),
			}
			left, err := createConnection(args[0], stdio)
			if err != nil {
				return err
			}
			defer left.Close()
			right, err := createConnection(args[1], stdio)
			if err != nil {
				return err
			}
			defer right.Close()
			return connect(left, right, uniDir)
		},
	}

	cmd.Flags().BoolVarP(&uniDir, "unidirect", "u", false, "unidirectional mode (left to right)")
	cmd.Flags().StringVarP(&dbg, "debug", "d", "", "log level (this flag is present only for compatibility and has no effect)")

	return &cmd
}

func createConnection(address string, stdio connection) (connection, error) {
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
		_, _ = fmt.Fprintln(os.Stderr, "opening connection")
		conn, err := net.Dial(typ, addr)
		if err == nil {
			_, _ = fmt.Fprintf(os.Stderr, "successfully connected to %v\n", conn.RemoteAddr())
		}
		return conn, err
	case "open":
		return os.OpenFile(addr, os.O_RDWR, 0644)
	default:
		return nil, fmt.Errorf("unsupported address: %q", address)
	}
}

// closeTimeout is how long connect keeps copying in one direction after the
// other has ended. It is the default of socat's -t option, though socat
// restarts it whenever data moves.
const closeTimeout = 500 * time.Millisecond

func connect(left, right connection, uniDir bool) error {
	errs := make(chan error, 2)
	pipe := func(dst, src connection) {
		_, err := io.Copy(dst, src)
		tryCloseWriteSide(dst)
		errs <- err
	}

	go pipe(right, left)
	if uniDir {
		return <-errs
	}
	go pipe(left, right)

	// Once one direction is done, give the other closeTimeout to finish,
	// then return so the caller closes both sides. The in-cluster dialer
	// sees a connection end only when socat exits, and its client keeps
	// stdin open, so waiting for both directions would hide a peer's close
	// from it.
	err := <-errs
	select {
	case err2 := <-errs:
		return errors.Join(err, err2)
	case <-time.After(closeTimeout):
		return err
	}
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
			fmt.Fprintf(os.Stderr, "warning: cannot close write side: %+v\n", err)
		}
	}
}
