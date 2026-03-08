package main

import (
	"fmt"
	"os"

	"github.com/ppiankov/contextspectre/internal/commands"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := commands.Execute(version, commit, date); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
