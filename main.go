package main

import (
	"bufio"
	"encoding/json"
	"fmt"
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
	"golang.org/x/term"
)

type Command struct {
	Command []string
	Width   uint16
	Height  uint16
}

func main() {
	// Start the server if the first argument is "start"
	if len(os.Args) > 1 && os.Args[1] == "start" {
		startServer()
		return
	}

	// Print help message if the first argument is "-h" or "--help"
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		log.Println(`Usage: hrun [options] command [args...]

If command is "start", it starts the server. Otherwise, it starts the client
and sends the command to the server. If no command is provided, it starts a
shell on the host.`)
		return
	}

	command := []string{"sh", "-c", os.Getenv("SHELL")}
	if len(os.Args) > 1 {
		command = os.Args[1:]
	}

	startClient(command)
}

func startServer() {
	// Create a listener for the server
	listener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	log.Printf("Server is running on 127.0.0.1:8080\n\n")

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
			go handleConnection(conn)
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

func handleConnection(conn net.Conn) {
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
	var cmdStruct Command
	if err := json.Unmarshal([]byte(rawCommand), &cmdStruct); err != nil {
		log.Printf("Error decoding command: %v", err)
		conn.Close()
		return
	}
	if len(cmdStruct.Command) == 0 {
		log.Println("No command provided")
		return
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

func startClient(command []string) {
	// Connect to the server
	conn, err := net.Dial("tcp", "127.0.0.1:8080")
	if err != nil {
		log.Println("Error connecting to the host:", err)
		return
	}
	defer conn.Close()

	// Get the initial terminal size
	initialWidth, initialHeight, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		log.Println("Error getting initial terminal size:", err)
		return
	}

	// Send the command to the server
	cmd := Command{
		Command: command,
		Width:   uint16(initialWidth),
		Height:  uint16(initialHeight),
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		log.Println("Error encoding command:", err)
		return
	}

	_, err = conn.Write(append(cmdBytes, '\n'))
	if err != nil {
		log.Println("Error sending command to the server:", err)
		return
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
		return
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
}
