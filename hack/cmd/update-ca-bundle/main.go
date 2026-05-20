package main

import (
	"context"
	"fmt"
	"os"

	"knative.dev/func/hack/cmd/shared"
)

const caBundleMakeTarget = "templates/certs/ca-certificates.crt"

func main() {
	ctx, stop := shared.NotifyContext(context.Background())
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	return shared.RunCmd(ctx, "make", caBundleMakeTarget)
}
