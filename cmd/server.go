package cmd

import (
	"github.com/containerpak/hrun/pkg/server"
)

func StartServer(allowedCmds []string, socketPath string) {
	server.StartServer(allowedCmds, socketPath)
}
