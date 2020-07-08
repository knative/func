package main

import (
	"github.com/boson-project/faas/cmd"
)

// Statically-populated build metadata set
// by `make build`.
var date, vers, hash string

func main() {
	cmd.SetMeta(date, vers, hash)
	cmd.Execute()
}
