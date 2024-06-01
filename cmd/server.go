package cmd

import (
	"github.com/distrobox/hrun/pkg/server"
)

func StartServer(allowedCmds []string, socketPath string) {
	server.StartServer(allowedCmds, socketPath)
}
