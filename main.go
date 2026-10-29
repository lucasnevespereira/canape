// Command canape controls the home from the couch. See internal/cli.
package main

import (
	"os"

	"github.com/lucasnevespereira/canape/internal/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:])) }
