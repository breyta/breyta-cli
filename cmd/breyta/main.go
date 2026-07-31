package main

import (
	"errors"
	"os"

	"github.com/breyta/breyta-cli/internal/cli"
)

func main() {
	cmd := cli.NewRootCmd()
	if err := cmd.Execute(); err != nil {
		var exitCoder cli.ExitCoder
		if errors.As(err, &exitCoder) && exitCoder.ExitCode() > 0 {
			os.Exit(exitCoder.ExitCode())
		}
		os.Exit(1)
	}
}
