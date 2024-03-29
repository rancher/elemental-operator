// Package main is a binary wrapper package around cmd.
package main

import (
	"os"

	"github.com/google/go-tpm-tools/cmd"
)

func main() {
	if cmd.RootCmd.Execute() != nil {
		os.Exit(1)
	}
}
