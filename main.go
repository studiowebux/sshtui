package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
)

// SSHHost represents a parsed SSH host
type SSHHost struct {
	Alias    string
	HostName string
	User     string
	Port     string
}

// Session represents a running SSH session with PTY
type Session struct {
	ID         int
	Alias      string
	Cmd        *exec.Cmd
	PTY        *os.File
	Active     bool
	Scrollback []byte // Captured output
}

var (
	sessions []*Session
	nextID   = 1
)

func main() {
	// Parse SSH config
	hosts, err := parseSSHConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Main loop
	for {
		showMenu(hosts)

		// Read choice
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "q" {
			// Kill all sessions
			for _, s := range sessions {
				if s.Cmd.Process != nil {
					s.Cmd.Process.Kill()
				}
				if s.PTY != nil {
					s.PTY.Close()
				}
			}
			break
		}

		if input == "x" {
			// Close last active session
			for i := len(sessions) - 1; i >= 0; i-- {
				if sessions[i].Active {
					if sessions[i].Cmd.Process != nil {
						sessions[i].Cmd.Process.Kill()
					}
					if sessions[i].PTY != nil {
						sessions[i].PTY.Close()
					}
					sessions = append(sessions[:i], sessions[i+1:]...)
					break
				}
			}
			continue
		}

		if input == "v" {
			// View scrollback
			if len(sessions) > 0 {
				fmt.Print("Which session? [!number]: ")
				reader := bufio.NewReader(os.Stdin)
				numStr, _ := reader.ReadString('\n')
				numStr = strings.TrimSpace(numStr)
				var num int
				if strings.HasPrefix(numStr, "!") {
					if _, err := fmt.Sscanf(numStr, "!%d", &num); err == nil {
						if num > 0 && num <= len(sessions) {
							viewScrollback(sessions[num-1])
						}
					}
				}
			}
			continue
		}

		// Check for session (!number) or host (number)
		if strings.HasPrefix(input, "!") {
			// Resume session
			var num int
			if _, err := fmt.Sscanf(input, "!%d", &num); err == nil {
				if num > 0 && num <= len(sessions) {
					attachToSession(sessions[num-1])
				}
			}
			continue
		}

		// Connect to new host
		var num int
		if _, err := fmt.Sscanf(input, "%d", &num); err == nil {
			if num > 0 && num <= len(hosts) {
				createSession(hosts[num-1])
			}
		}
	}
}

func parseSSHConfig() ([]SSHHost, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(home, ".ssh", "config")
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var hosts []SSHHost
	var current *SSHHost

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		key := strings.ToLower(parts[0])
		value := strings.Join(parts[1:], " ")

		if key == "host" {
			if strings.Contains(value, "*") {
				current = nil
				continue
			}

			if current != nil {
				hosts = append(hosts, *current)
			}

			current = &SSHHost{Alias: value}
			continue
		}

		if current == nil {
			continue
		}

		switch key {
		case "hostname":
			current.HostName = value
		case "user":
			current.User = value
		case "port":
			current.Port = value
		}
	}

	if current != nil {
		hosts = append(hosts, *current)
	}

	return hosts, scanner.Err()
}

func showMenu(hosts []SSHHost) {
	fmt.Print("\033[2J\033[H") // Clear screen
	fmt.Println("╔════════════════════════════════════════╗")
	fmt.Println("║    sshtui - Session Manager (v2)       ║")
	fmt.Println("╚════════════════════════════════════════╝\n")

	if len(sessions) > 0 {
		fmt.Println("Active Sessions:")
		for i, s := range sessions {
			status := "alive"
			if s.Cmd.ProcessState != nil && s.Cmd.ProcessState.Exited() {
				status = "ended"
			}
			fmt.Printf("  [!%d] %s (%s)\n", i+1, s.Alias, status)
		}
		fmt.Println()
	}

	fmt.Println("Connections:")
	for i, host := range hosts {
		fmt.Printf("  [%d] %s", i+1, host.Alias)
		if host.HostName != "" {
			fmt.Printf(" (%s)", host.HostName)
		}
		fmt.Println()
	}

	fmt.Println("\nCommands:")
	fmt.Println("  [number]  - Connect to host")
	fmt.Println("  [!number] - Resume session")
	fmt.Println("  v         - View scrollback/history")
	fmt.Println("  x         - Close active session")
	fmt.Println("  q         - Quit all")
	fmt.Println("\nIn session: Ctrl+Space to detach")
	fmt.Print("\n> ")
}

func createSession(host SSHHost) {
	fmt.Printf("\nConnecting to %s...\n", host.Alias)

	cmd := exec.Command("ssh", host.Alias)

	// Start with PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		fmt.Printf("Error: %v\nPress Enter...", err)
		bufio.NewReader(os.Stdin).ReadString('\n')
		return
	}

	session := &Session{
		ID:     nextID,
		Alias:  host.Alias,
		Cmd:    cmd,
		PTY:    ptmx,
		Active: true,
	}
	nextID++

	sessions = append(sessions, session)

	// Monitor session
	go func() {
		cmd.Wait()
		session.Active = false
	}()

	// Attach immediately
	attachToSession(session)
}

func attachToSession(session *Session) {
	if session.Cmd.ProcessState != nil && session.Cmd.ProcessState.Exited() {
		fmt.Println("Session has ended. Press Enter...")
		bufio.NewReader(os.Stdin).ReadString('\n')
		return
	}

	fmt.Print("\033[2J\033[H") // Clear
	fmt.Printf("╔════════════════════════════════════════╗\n")
	fmt.Printf("║ Connected: %-28s║\n", session.Alias)
	fmt.Printf("║ Ctrl+Space to detach                   ║\n")
	fmt.Printf("╚════════════════════════════════════════╝\n\n")

	// Replay scrollback buffer when reattaching
	if len(session.Scrollback) > 0 {
		// Show last portion of scrollback (last 50 lines or 4KB)
		scrollbackToShow := session.Scrollback

		// Limit to last 4KB to avoid flooding terminal
		if len(scrollbackToShow) > 4096 {
			scrollbackToShow = scrollbackToShow[len(scrollbackToShow)-4096:]
		}

		// Write scrollback to stdout
		os.Stdout.Write(scrollbackToShow)
		fmt.Println("\n--- [Scrollback end, live session resumed] ---")
	}

	// Set PTY size
	if ws, err := pty.GetsizeFull(os.Stdin); err == nil {
		pty.Setsize(session.PTY, ws)
	}

	// Handle window resize
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			if ws, err := pty.GetsizeFull(os.Stdin); err == nil {
				pty.Setsize(session.PTY, ws)
			}
		}
	}()
	defer signal.Stop(winch)

	// Set raw mode
	oldState, err := makeRaw(os.Stdin.Fd())
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer restore(os.Stdin.Fd(), oldState)

	// I/O proxy with Ctrl+D detection
	done := make(chan bool, 2)

	// Stdin -> PTY
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				done <- true
				return
			}

			// Check for Ctrl+Space (ASCII 0)
			for i := 0; i < n; i++ {
				if buf[i] == 0 {
					done <- true
					return
				}
			}

			_, err = session.PTY.Write(buf[:n])
			if err != nil {
				done <- true
				return
			}
		}
	}()

	// PTY -> Stdout (with capture to scrollback)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := session.PTY.Read(buf)
			if err != nil {
				done <- true
				return
			}
			if n > 0 {
				// Write to stdout
				os.Stdout.Write(buf[:n])

				// Append to scrollback
				session.Scrollback = append(session.Scrollback, buf[:n]...)

				// Keep scrollback reasonable (last 1MB)
				if len(session.Scrollback) > 1024*1024 {
					session.Scrollback = session.Scrollback[len(session.Scrollback)-1024*1024:]
				}
			}
		}
	}()

	// Wait for detach or end
	<-done

	restore(os.Stdin.Fd(), oldState)
	fmt.Print("\n\n[Detached - Press Enter]\n")
	bufio.NewReader(os.Stdin).ReadString('\n')
}

func makeRaw(fd uintptr) (*syscall.Termios, error) {
	var oldState syscall.Termios
	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, fd, syscall.TIOCGETA, uintptr(unsafe.Pointer(&oldState)), 0, 0, 0); err != 0 {
		return nil, err
	}

	newState := oldState
	newState.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	newState.Oflag &^= syscall.OPOST
	newState.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	newState.Cflag &^= syscall.CSIZE | syscall.PARENB
	newState.Cflag |= syscall.CS8

	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, fd, syscall.TIOCSETA, uintptr(unsafe.Pointer(&newState)), 0, 0, 0); err != 0 {
		return nil, err
	}

	return &oldState, nil
}

func restore(fd uintptr, state *syscall.Termios) error {
	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, fd, syscall.TIOCSETA, uintptr(unsafe.Pointer(state)), 0, 0, 0); err != 0 {
		return err
	}
	return nil
}

func viewScrollback(session *Session) {
	if len(session.Scrollback) == 0 {
		fmt.Println("No scrollback available. Press Enter...")
		bufio.NewReader(os.Stdin).ReadString('\n')
		return
	}

	fmt.Print("\033[2J\033[H") // Clear
	fmt.Printf("╔════════════════════════════════════════╗\n")
	fmt.Printf("║ Scrollback: %-27s║\n", session.Alias)
	fmt.Printf("║ Commands: /search, n next, q quit      ║\n")
	fmt.Printf("╚════════════════════════════════════════╝\n\n")

	// Split into lines
	lines := strings.Split(string(session.Scrollback), "\n")
	currentLine := 0
	pageSize := 20
	searchTerm := ""
	searchResults := []int{}
	searchIndex := -1

	reader := bufio.NewReader(os.Stdin)

	for {
		// Display current page
		fmt.Print("\033[2J\033[H")
		fmt.Printf("╔════════════════════════════════════════╗\n")
		fmt.Printf("║ Scrollback: %-27s║\n", session.Alias)
		if searchTerm != "" {
			fmt.Printf("║ Search: %-31s║\n", searchTerm)
			fmt.Printf("║ Matches: %-30d║\n", len(searchResults))
		}
		fmt.Printf("╚════════════════════════════════════════╝\n\n")

		endLine := currentLine + pageSize
		if endLine > len(lines) {
			endLine = len(lines)
		}

		for i := currentLine; i < endLine; i++ {
			line := lines[i]
			// Highlight search term
			if searchTerm != "" && strings.Contains(strings.ToLower(line), strings.ToLower(searchTerm)) {
				line = strings.ReplaceAll(line, searchTerm, "\033[7m"+searchTerm+"\033[0m")
			}
			fmt.Println(line)
		}

		fmt.Printf("\n[Line %d/%d] ", currentLine, len(lines))
		if searchTerm != "" && len(searchResults) > 0 {
			fmt.Printf("[Match %d/%d] ", searchIndex+1, len(searchResults))
		}
		fmt.Print("Command: ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		switch {
		case input == "q":
			return

		case input == "j" || input == "":
			// Scroll down
			if currentLine+pageSize < len(lines) {
				currentLine += pageSize
			}

		case input == "k":
			// Scroll up
			if currentLine-pageSize >= 0 {
				currentLine -= pageSize
			} else {
				currentLine = 0
			}

		case input == "g":
			// Go to top
			currentLine = 0

		case input == "G":
			// Go to bottom
			if len(lines) > pageSize {
				currentLine = len(lines) - pageSize
			}

		case strings.HasPrefix(input, "/"):
			// Search
			searchTerm = strings.TrimPrefix(input, "/")
			searchResults = []int{}
			for i, line := range lines {
				if strings.Contains(strings.ToLower(line), strings.ToLower(searchTerm)) {
					searchResults = append(searchResults, i)
				}
			}
			if len(searchResults) > 0 {
				searchIndex = 0
				currentLine = searchResults[0]
			}

		case input == "n":
			// Next search result
			if len(searchResults) > 0 {
				searchIndex = (searchIndex + 1) % len(searchResults)
				currentLine = searchResults[searchIndex]
			}

		case input == "N":
			// Previous search result
			if len(searchResults) > 0 {
				searchIndex = (searchIndex - 1 + len(searchResults)) % len(searchResults)
				currentLine = searchResults[searchIndex]
			}
		}
	}
}
