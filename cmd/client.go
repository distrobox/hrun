package cmd

import (
	"github.com/distrobox/hrun/pkg/client"
)

func StartClient(command []string, socketPath string) {
	client.StartClient(command, socketPath)
}
