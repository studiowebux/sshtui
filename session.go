package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
)

const (
	ScrollbackReplaySize = 4096
	MaxScrollbackSize    = 1024 * 1024
	StdinBufSize         = 1024
	PtyBufSize           = 4096
)

// Session represents a running SSH session with PTY
type Session struct {
	ID         int
	Alias      string
	Cmd        *exec.Cmd
	PTY        *os.File
	Active     bool
	Scrollback []byte
}

var (
	sessions   []*Session
	nextID     = 1
	sessionsMu sync.RWMutex
)

func createSession(host SSHHost) {
	fmt.Printf("\nConnecting to %s...\n", host.Alias)

	args := buildSSHArgs(host)
	cmd := exec.Command("ssh", args...)

	// Start with PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		fmt.Printf("Error: %v\nPress Enter...", err)
		bufio.NewReader(os.Stdin).ReadString('\n')
		return
	}

	sessionsMu.Lock()
	session := &Session{
		ID:     nextID,
		Alias:  host.Alias,
		Cmd:    cmd,
		PTY:    ptmx,
		Active: true,
	}
	nextID++
	sessions = append(sessions, session)
	sessionsMu.Unlock()

	// Monitor session
	go func() {
		cmd.Wait()
		sessionsMu.Lock()
		session.Active = false
		sessionsMu.Unlock()
	}()

	// Attach immediately
	attachToSession(session)
}

func attachToSession(session *Session) {
	// Panic recovery to ensure terminal is restored
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("\n\nPanic recovered: %v\n", r)
			fmt.Println("Terminal state restored. Press Enter...")
			bufio.NewReader(os.Stdin).ReadString('\n')
		}
	}()

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
		scrollbackToShow := session.Scrollback

		// Limit to last 4KB to avoid flooding terminal
		if len(scrollbackToShow) > ScrollbackReplaySize {
			scrollbackToShow = scrollbackToShow[len(scrollbackToShow)-ScrollbackReplaySize:]
		}

		// Write scrollback to stdout
		os.Stdout.Write(scrollbackToShow)
		fmt.Println("\n--- [Scrollback end, live session resumed] ---")
	}

	// Set PTY size
	if ws, err := pty.GetsizeFull(os.Stdin); err == nil {
		pty.Setsize(session.PTY, ws)
	}

	// Handle window resize with proper cleanup
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-winch:
				if ws, err := pty.GetsizeFull(os.Stdin); err == nil {
					pty.Setsize(session.PTY, ws)
				}
			case <-done:
				return
			}
		}
	}()

	defer func() {
		signal.Stop(winch)
		close(done)
	}()

	// Set raw mode
	oldState, err := makeRaw(os.Stdin.Fd())
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer restore(os.Stdin.Fd(), oldState)

	// I/O proxy
	ioStop := make(chan bool, 1)

	// Stdin -> PTY
	go func() {
		buf := make([]byte, StdinBufSize)
		for {
			select {
			case <-ioStop:
				return
			default:
				n, err := os.Stdin.Read(buf)
				if err != nil {
					ioStop <- true
					return
				}

				// Check for Ctrl+Space (ASCII 0)
				for i := 0; i < n; i++ {
					if buf[i] == 0 {
						ioStop <- true
						return
					}
				}

				_, err = session.PTY.Write(buf[:n])
				if err != nil {
					ioStop <- true
					return
				}
			}
		}
	}()

	// PTY -> Stdout (with capture to scrollback)
	go func() {
		buf := make([]byte, PtyBufSize)
		for {
			select {
			case <-ioStop:
				return
			default:
				n, err := session.PTY.Read(buf)
				if err != nil {
					ioStop <- true
					return
				}
				if n > 0 {
					// Write to stdout
					os.Stdout.Write(buf[:n])

					// Append to scrollback
					session.Scrollback = append(session.Scrollback, buf[:n]...)

					// Keep scrollback reasonable (last 1MB)
					if len(session.Scrollback) > MaxScrollbackSize {
						session.Scrollback = session.Scrollback[len(session.Scrollback)-MaxScrollbackSize:]
					}
				}
			}
		}
	}()

	// Wait for detach or end
	<-ioStop

	// Give goroutines time to exit (they may be blocked on Read)
	// Terminal state is restored by defer, so stdin becomes line-buffered again
	// The user's Enter key press will be consumed by either:
	// 1. The lingering goroutine (harmless), or
	// 2. This ReadString (intended)
	fmt.Print("\n\n[Detached]\n")
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

func closeAllSessions() {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	for _, s := range sessions {
		if s.PTY != nil {
			s.PTY.Close()
		}
		if s.Cmd.Process != nil {
			s.Cmd.Process.Kill()
			s.Cmd.Wait()
		}
	}
}

func closeActiveSession() {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	for i := len(sessions) - 1; i >= 0; i-- {
		if sessions[i].Active {
			if sessions[i].PTY != nil {
				sessions[i].PTY.Close()
			}
			if sessions[i].Cmd.Process != nil {
				sessions[i].Cmd.Process.Kill()
				sessions[i].Cmd.Wait()
			}
			sessions = append(sessions[:i], sessions[i+1:]...)
			break
		}
	}
}
