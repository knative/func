package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/boson-project/func/cmd"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
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

	cmd.SetMeta(date, vers, hash)
	cmd.Execute(ctx)
}
