package cmd

import (
	"fmt"
	"os"
)

func ShowHelp() {
	fmt.Fprintf(os.Stderr, `Usage: hrun [options] [command] [args...]

Options:
  -h, --help         Display this help message.
  --start            Start the server.
  --allowed-cmd      Specify allowed command (can be used multiple times).
  --socket           Specify an alternative socket path (default: /tmp/hrun.sock).

If command is "start", it starts the server with specified allowed commands.
Otherwise, it starts the client and sends the command to the server.
If no command is provided, it starts a shell on the host.
`)
	os.Exit(0)
}
