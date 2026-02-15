package main

import (
	"os"

	"github.com/jeffvincent/kindling/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
