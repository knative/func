package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"knative.dev/kn-plugin-func/cmd"
)

// Statically-populated build metadata set
// by `make build`.
var date, vers, hash string

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		cancel()
		// second sigint/sigterm is treated as sigkill
		<-sigs
		os.Exit(137)
	}()

	root, err := cmd.NewRootCmd(cmd.RootCommandConfig{
		Name:    "func",
		Date:    date,
		Version: vers,
		Hash:    hash,
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := root.ExecuteContext(ctx); err != nil {
		if ctx.Err() != nil {
			os.Exit(130)
			return
		}
		// Errors are printed to STDERR output and the process exits with code of 1.
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
