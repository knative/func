package main

import (
	"fmt"
	"os"
)

func main() {
	// here is where you delegate to different files in the hack/ directory.

	// If youre creating a new script, just:
	// 	Set an identifying string to the switch below along with your function
	// 	which will be an entry to your script - similar to others (dont forget to
	// 	add that string as a cli argument when calling main.go here)

	var err error
	args := os.Args[1:]
	if len(args) > 1 {
		fmt.Fprintf(os.Stderr, "too many arguments '%v'\n", args)
		os.Exit(1)
	}
	arg := args[0]
	switch arg {
	case "update-builder":
		updateBuilder()
	case "update-components":
		err = updateComponentVersions()
	default:
		fmt.Fprintf(os.Stderr, "unknown argument '%s', don't know which hack/ script to run", arg)
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
