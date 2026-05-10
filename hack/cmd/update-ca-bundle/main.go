package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"knative.dev/func/hack/cmd/shared"
)

const caBundleMakeTarget = "templates/certs/ca-certificates.crt"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
		<-sigs
		os.Exit(130)
	}()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	return shared.RunCmd(ctx, "make", caBundleMakeTarget)
}
