package server

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"github.com/distrobox/hrun/pkg/structs"
)

func StartServer(allowedCmds []string, socketPath string) {
	// Create a listener for the server
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	log.Printf("Server is running on %s\n", listener.Addr())

	// Set up a signal handler to shut down the server
	doneCh := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

		<-sigCh
		log.Println("Shutdown signal received, closing server...")
		close(doneCh)
	}()

	// Accept connections and handle them
	for {
		select {
		case <-doneCh:
			log.Println("Shutting down server...")
			return
		case conn, ok := <-acceptConn(listener):
			if !ok {
				log.Println("Listener closed, shutting down server...")
				return
			}
			go handleConnection(conn, allowedCmds)
		}
	}
}

func acceptConn(listener net.Listener) <-chan net.Conn {
	ch := make(chan net.Conn)
	go func() {
		defer close(ch)
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			return
		}
		ch <- conn
	}()
	return ch
}

func handleConnection(conn net.Conn, allowedCmds []string) {
	defer conn.Close()

	// Read the command from the client
	reader := bufio.NewReader(conn)
	rawCommand, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Failed to read command: ", err)
		return
	}
	log.Printf("Received command: %s", rawCommand)

	// Decode the command into the Command struct
	var cmdStruct structs.Command
	if err := json.Unmarshal([]byte(rawCommand), &cmdStruct); err != nil {
		log.Printf("Error decoding command: %v", err)
		conn.Close()
		return
	}
	if len(cmdStruct.Command) == 0 {
		log.Println("No command provided")
		return
	}

	// Check if the command is allowed
	if len(allowedCmds) > 0 {
		allowed := false
		for _, allowedCmd := range allowedCmds {
			if cmdStruct.Command[0] == allowedCmd {
				allowed = true
				break
			}
		}
		if !allowed {
			log.Printf("Command %s is not allowed", cmdStruct.Command[0])
			conn.Close()
			return
		}
	}

	// Prepare a pty
	var ptyMaster, ptySlave *os.File
	ptyMaster, ptySlave, err = pty.Open()
	if err != nil {
		log.Println("Error creating PTY:", err)
		conn.Close()
		return
	}
	defer ptySlave.Close()
	log.Println("PTY created")

	// Set initial terminal size
	ws := &pty.Winsize{
		Cols: cmdStruct.Width,
		Rows: cmdStruct.Height,
	}
	if err := pty.Setsize(ptyMaster, ws); err != nil {
		log.Printf("Error setting initial terminal size: %v", err)
	} else {
		log.Printf("Terminal initialized to %dx%d", cmdStruct.Width, cmdStruct.Height)
	}

	// Set up the channels to communicate with the host
	go func() {
		io.Copy(conn, ptyMaster)
		ptyMaster.Close()
		conn.Close()
	}()
	go func() {
		io.Copy(ptyMaster, conn)
		ptyMaster.Close()
		conn.Close()
	}()

	// Set the terminal size on resize request
	go func() {
		for {
			message, err := reader.ReadString('\n')
			if err != nil {
				break
			}

			if strings.HasPrefix(message, "resize:") {
				log.Println("Resize request received")
				trimmedMessage := strings.TrimSpace(message)
				parts := strings.Split(trimmedMessage, ":")
				if len(parts) == 3 {
					width, errWidth := strconv.Atoi(parts[1])
					height, errHeight := strconv.Atoi(parts[2])
					if errWidth != nil || errHeight != nil {
						log.Printf("Error converting dimensions to integers: width error %v, height error %v", errWidth, errHeight)
						continue
					}
					ws := &pty.Winsize{
						Cols: uint16(width),
						Rows: uint16(height),
					}
					if err := pty.Setsize(ptyMaster, ws); err != nil {
						log.Printf("Error resizing PTY: %v", err)
					} else {
						log.Printf("Terminal resized to %dx%d", width, height)
					}
				} else {
					log.Println("Invalid resize message format")
				}
			}
		}
	}()

	// Execute the command
	cmd := exec.Command(cmdStruct.Command[0], cmdStruct.Command[1:]...)
	cmd.Stdin = ptySlave
	cmd.Stdout = ptySlave
	cmd.Stderr = ptySlave

	// Set the process attributes
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setctty:   true,
		Setsid:    true,
		Pdeathsig: syscall.SIGTERM,
	}

	// Start the shell process
	if err = cmd.Start(); err != nil {
		log.Println("Error starting shell:", err)
		return
	}
	log.Println("Shell started")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	// Handle the termination signal
	go func() {
		<-sigCh
		conn.Close()
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}()

	// Wait for the shell process to exit
	cmd.Wait()
	log.Println("Shell process exited")
	log.Printf("Connection closed\n\n")
}
