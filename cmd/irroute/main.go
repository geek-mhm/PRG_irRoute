package main

import (
	"fmt"
	"os"

	"irroute/internal/app"
	"irroute/internal/store"
	sys "irroute/internal/system"
)

var version = "0.1.2-dev"

func main() {
	root := os.Getenv("IRROUTE_ROOT")
	dataStore := store.New(store.NewPaths(root))
	runner := sys.HostRunner{}
	application := app.New(os.Stdin, os.Stdout, os.Stderr, dataStore, runner, version)
	if err := application.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
