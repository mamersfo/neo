package main

import (
	"os"

	"github.com/mamersfo/neo/cmd"
)

var version = "0.1.0-dev"

func main() {
	os.Exit(cmd.Execute(version))
}
