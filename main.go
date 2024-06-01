package main

import (
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/distrobox/hrun/cmd"
)

func main() {
	helpFlag := flag.Bool("h", false, "Display help")
	helpFlagLong := flag.Bool("help", false, "Display help")
	startFlag := flag.Bool("start", false, "Start the server")
	socketFlag := flag.String("socket", "/tmp/hrun.sock", "Specify an alternative socket path")
	allowedCmds := make([]string, 0)
	flag.Func("allowed-cmd", "Specify allowed command (can be used multiple times)", func(cmd string) error {
		allowedCmds = append(allowedCmds, cmd)
		return nil
	})

	flag.Usage = func() {
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
	}

	flag.Parse()

	// Help message
	if *helpFlag || *helpFlagLong {
		cmd.ShowHelp()
		return
	}

	// Server mode
	if *startFlag {
		cmd.StartServer(allowedCmds, *socketFlag)
		return
	}

	// Client mode
	var command []string
	if path.Base(os.Args[0]) == "hrun" && len(flag.Args()) == 0 {
		command = []string{"sh", "-c", os.Getenv("SHELL")}
	} else {
		command = flag.Args()
	}

	cmd.StartClient(command, *socketFlag)
}
