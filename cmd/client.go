package cmd

import (
	"github.com/containerpak/hrun/pkg/client"
)

func StartClient(command []string, socketPath string) {
	client.StartClient(command, socketPath)
}
