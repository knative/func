package cmd

import (
	"fmt"
	"os"
)

// Execute the command tree by executing the root command, which runs
// according to the context defined by:  the optional config file,
// Environment Variables, command arguments and flags.
func Execute() {
	// Execute the root of the command tree.
	if err := root.Execute(); err != nil {
		// Errors are printed to STDERR output and the process exits with code of 1.
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
