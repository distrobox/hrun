package main

import (
	"bufio"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

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

	// Start the client otherwise
	command := "sh -c $SHELL"
	if len(os.Args) > 1 {
		command = strings.Join(os.Args[1:], " ")
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

	// Read the command from the client and split it into parts
	reader := bufio.NewReader(conn)
	command, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Failed to read command: ", err)
		return
	}
	log.Printf("Received command: %s", command)
	command = strings.TrimSpace(command)
	cmdParts := strings.Fields(command)
	if len(cmdParts) == 0 {
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
	log.Println("PTY created")

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

	// Execute the command
	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
	cmd.Stdin = ptySlave
	cmd.Stdout = ptySlave
	cmd.Stderr = ptySlave

	// Set the process group so that the termination signal is
	// forwarded to the shell process
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Setctty = true
	cmd.SysProcAttr.Setsid = true
	cmd.SysProcAttr.Pdeathsig = syscall.SIGTERM

	// Start the shell process
	if err = cmd.Start(); err != nil {
		log.Println("Error starting shell:", err)
		conn.Close()
		return
	}
	log.Println("Shell started")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	// Setting up a goroutine to wait for the shell process to exit
	go func() {
		cmd.Wait()
		ptySlave.Close()
	}()

	// Handle the termination signal
	go func() {
		<-sigCh
		log.Println("Closing the connection and shell process...")
		conn.Close()
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}()

	// Wait for the shell process to exit
	cmd.Wait()
	log.Println("Shell process exited")
	ptySlave.Close()
	log.Printf("Connection closed\n\n")
}

func startClient(command string) {
	// Connect to the server
	conn, err := net.Dial("tcp", "127.0.0.1:8080")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// Send the command to the server
	_, err = conn.Write([]byte(command + "\n"))
	if err != nil {
		log.Println("Error sending command to the server:", err)
		return
	}

	// Set the terminal to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
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
