// Command fastsub enumerates subdomains and probes which of them are alive.
package main

import (
	"os"

	"github.com/guaidao2/fastsub/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
