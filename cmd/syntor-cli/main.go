package main

import (
	"fmt"
	"os"

	"github.com/syntor/syntor/pkg/devtools"
)

func main() {
	cli := devtools.NewCLI()
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
