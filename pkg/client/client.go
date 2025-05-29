package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"

	"syscall"

	"github.com/containerpak/hrun/pkg/structs"
	"golang.org/x/term"
)

func StartClient(command []string, socketPath string) error {
	// Connect to the server
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		log.Println("Error connecting to the host:", err)
		return errors.New("failed to connect to the server")
	}
	defer conn.Close()

	// Get the initial terminal size
	initialWidth, initialHeight, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		log.Println("Error getting initial terminal size:", err)
		return errors.New("failed to get terminal size")
	}

	// Send the command to the server
	cmd := structs.Command{
		Command: command,
		Width:   uint16(initialWidth),
		Height:  uint16(initialHeight),
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		log.Println("Error encoding command:", err)
		return errors.New("failed to encode command")
	}

	_, err = conn.Write(append(cmdBytes, '\n'))
	if err != nil {
		log.Println("Error sending command to the server:", err)
		return errors.New("failed to send command to the server")
	}

	// Set up handling for SIGWINCH (window change) signal to detect terminal resize events
	sendTerminalSize := func() {
		width, height, err := term.GetSize(int(os.Stdin.Fd()))
		if err != nil {
			log.Println("Error getting terminal size:", err)
			return
		}

		resizeCommand := fmt.Sprintf("resize:%d:%d\n", width, height)
		_, err = conn.Write([]byte(resizeCommand))
		if err != nil {
			log.Println("Error sending terminal size to the server:", err)
		}
	}

	sigwinchChan := make(chan os.Signal, 1)
	signal.Notify(sigwinchChan, syscall.SIGWINCH)
	go func() {
		for range sigwinchChan {
			sendTerminalSize()
		}
	}()

	// Set the terminal to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Println("Error setting terminal to raw mode:", err)
		return errors.New("failed to set terminal to raw mode")
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	// Create a channel to communicate with the pty
	doneCh := make(chan struct{})
	go func() {
		_, err := io.Copy(conn, os.Stdin)
		if err != nil {
			log.Println("Error copying data to the server:", err)
		}
		close(doneCh)
	}()
	go func() {
		_, err := io.Copy(os.Stdout, conn)
		if err != nil {
			log.Println("Error copying data from the server:", err)
		}
		close(doneCh)
	}()

	<-doneCh

	// Clean up
	signal.Stop(sigwinchChan)
	close(sigwinchChan)
	_ = term.Restore(int(os.Stdin.Fd()), oldState)
	_ = conn.Close()
	log.Println("Client connection closed")
	return nil
}
