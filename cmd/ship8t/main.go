// Command ship8t is the Ship8t client.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/ship8t/ship8t/internal/version"
)

const (
	exitSuccess    = 0
	exitUsageError = 2
)

const usageText = `Usage: ship8t <command>

Commands:
  version    Print the client version

Run "ship8t help" to show this message.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes one client command and returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return exitUsageError
	}

	command := args[0]
	switch command {
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usageText)
		return exitSuccess
	case "version":
		fmt.Fprintf(stdout, "ship8t %s\n", version.Current())
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "ship8t: unknown command %q\n\n%s", command, usageText)
		return exitUsageError
	}
}
