package main

import (
	"os"

	"github.com/alexdancer/skillglass/internal/terminal"
)

func main() {
	os.Exit(terminal.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
