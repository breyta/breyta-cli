package main

import (
        "os"

        "breyta-cli/internal/cli"
)

func main() {
        cmd := cli.NewRootCmd()
        if err := cmd.Execute(); err != nil {
                os.Exit(1)
        }
}
