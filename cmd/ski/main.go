package main

import (
	"fmt"
	"os"

	"ski/internal/cli"
)

func main() {
	cmd := cli.NewRootCmd(cli.Options{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
