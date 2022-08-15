package main

import (
	"log"

	"github.com/spf13/cobra/doc"
	"knative.dev/kn-plugin-func/cmd"
)

// This helper application generates markdown help documents
func main() {
	var root = cmd.NewRootCmd(cmd.RootCommandConfig{Name: "func"})
	if err := doc.GenMarkdownTree(root, "docs/reference"); err != nil {
		log.Fatal(err)
	}
}
