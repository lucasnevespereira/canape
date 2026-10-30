// Command canape controls the home from the couch. See cmd.
package main

import (
	"os"

	"github.com/lucasnevespereira/canape/cmd"
)

func main() { os.Exit(cmd.Run(os.Args[1:])) }
