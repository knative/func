package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"knative.dev/kn-plugin-func/cmd"
)

// Statically-populated build metadata set by `make build`.
var date, vers, hash string

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
		<-sigs // second sigint/sigterm is treated as sigkill
		os.Exit(137)
	}()

	version := cmd.Version{
		Date: date,
		Vers: vers,
		Hash: hash,
	}

	if err := cmd.NewRootCmd("func", version).ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		if ctx.Err() != nil {
			os.Exit(130)
		}
		os.Exit(1)
	}
}
