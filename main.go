package main

import (
	"bufio"
	"fmt"
	"io"
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
		fmt.Println(`Usage: hrun [options] command [args...]

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
	// create a listener for the server
	listener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	fmt.Println("Server is running on 127.0.0.1:8080")

	// accept connections from the client
	for {
		var conn net.Conn
		conn, err = listener.Accept()
		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			continue
		}

		defer conn.Close()

		// read the command from the client and split it into parts
		reader := bufio.NewReader(conn)
		command, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read command: ", err)
			return
		}
		fmt.Printf("Received command: %s\n", command)
		command = strings.TrimSpace(command)
		cmdParts := strings.Fields(command)
		if len(cmdParts) == 0 {
			fmt.Fprintf(conn, "Invalid command\n")
			return
		}

		// prepare a pty
		var ptyMaster, ptySlave *os.File
		ptyMaster, ptySlave, err = pty.Open()
		if err != nil {
			fmt.Println("Error creating PTY:", err)
			conn.Close()
			return
		}
		fmt.Println("PTY created")

		// set up the channels to communicate with the host
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
		fmt.Println("Channels set up")

		// execute the command
		cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
		cmd.Stdin = ptySlave
		cmd.Stdout = ptySlave
		cmd.Stderr = ptySlave

		// set the process group so that the termination signal is
		// forwarded to the shell process
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Setctty = true
		cmd.SysProcAttr.Setsid = true
		cmd.SysProcAttr.Pdeathsig = syscall.SIGTERM

		// start the shell process
		if err = cmd.Start(); err != nil {
			fmt.Println("Error starting shell:", err)
			conn.Close()
			return
		}
		fmt.Println("Shell started")

		// we need to create a channel to handle termination signals
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

		// setting up a goroutine to wait for the shell process to exit
		go func() {
			cmd.Wait()
			ptySlave.Close()
		}()

		// a goroutine to handle termination signals
		go func() {
			<-sigCh
			fmt.Println("Closing the connection and shell process...")
			conn.Close()
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}()

		// wait for the shell process to exit
		cmd.Wait()
		fmt.Println("Shell process exited")
	}
}

func startClient(command string) {
	// connect to the server
	conn, err := net.Dial("tcp", "127.0.0.1:8080")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// send the command to the server
	_, err = conn.Write([]byte(command + "\n"))
	if err != nil {
		fmt.Println("Error sending command to the server:", err)
		return
	}

	// set the terminal to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	// create a channel to communicate with the pty
	doneCh := make(chan struct{})
	go func() {
		_, err := io.Copy(conn, os.Stdin)
		if err != nil {
			fmt.Println("Error copying data to the server:", err)
		}
		close(doneCh)
	}()
	go func() {
		_, err := io.Copy(os.Stdout, conn)
		if err != nil {
			fmt.Println("Error copying data from the server:", err)
		}
		close(doneCh)
	}()

	<-doneCh
}
