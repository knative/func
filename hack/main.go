package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// here is where you delegate to different files in the hack/ directory.

	// If youre creating a new script, just:
	// 	Set an identifying string to the switch below along with your function
	// 	which will be an entry to your script - similar to others (dont forget to
	// 	add that string as a cli argument when calling main.go here)

	var err error
	args := os.Args[1:]
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "expected exactly 1 argument: '%v'\n", args)
		os.Exit(1)
	}

	// Set up context for possible signal inputs to not disrupt cleanup process.
	// This is not gonna do much for workflows since they finish and shutdown
	// but in case of local testing - dont leave left over resources on disk/RAM.
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

	switch args[0] {
	case "update-builder":
		err = updateBuilder(ctx)
	case "update-components":
		err = updateComponentVersions(ctx)
	default:
		fmt.Fprintf(os.Stderr, "unknown argument '%s', don't know which hack/ script to run", args[0])
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	// TODO: gauron99 - rewrite in go
	// TODO: update-quarkus-platform.js
	// TODO: update-springboot-platform.js
}
