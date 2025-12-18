package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const (
	ScrollbackReplaySize = 4096
	MaxScrollbackSize    = 1024 * 1024
	StdinBufSize         = 1024
	PtyBufSize           = 4096
	ConnectionTimeout    = 10 * time.Second
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

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), ConnectionTimeout)
	defer cancel()

	// Start with PTY in goroutine to support timeout
	type ptyResult struct {
		ptmx *os.File
		err  error
	}
	resultCh := make(chan ptyResult, 1)

	go func() {
		ptmx, err := pty.Start(cmd)
		resultCh <- ptyResult{ptmx: ptmx, err: err}
	}()

	// Wait for connection or timeout
	var ptmx *os.File
	var err error
	select {
	case result := <-resultCh:
		ptmx = result.ptmx
		err = result.err
	case <-ctx.Done():
		// Timeout occurred
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		fmt.Printf("Connection timeout after %v\nPress Enter...", ConnectionTimeout)
		bufio.NewReader(os.Stdin).ReadString('\n')
		return
	}

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
	ioStop := make(chan bool, 2) // Buffered to avoid blocking goroutines

	// Stdin -> PTY
	go func() {
		buf := make([]byte, StdinBufSize)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				select {
				case ioStop <- true:
				default:
				}
				return
			}

			// Check for Ctrl+Space (ASCII 0)
			for i := 0; i < n; i++ {
				if buf[i] == 0 {
					select {
					case ioStop <- true:
					default:
					}
					return
				}
			}

			_, err = session.PTY.Write(buf[:n])
			if err != nil {
				select {
				case ioStop <- true:
				default:
				}
				return
			}
		}
	}()

	// PTY -> Stdout (with capture to scrollback)
	go func() {
		buf := make([]byte, PtyBufSize)
		for {
			n, err := session.PTY.Read(buf)
			if err != nil {
				select {
				case ioStop <- true:
				default:
				}
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
	}()

	// Wait for detach or end
	<-ioStop

	// Don't close the channel - goroutines might still try to send to it
	// Just let them finish naturally or remain blocked on Read()

	// Note: Goroutines may still be blocked on Read() calls.
	// Terminal restoration via defer will make stdin line-buffered again.
	// The drainStdin call below will consume any pending input.

	// Drain any pending stdin input that may have accumulated
	// This prevents the need for double Enter after detach
	drainStdin()

	fmt.Print("\n\n[Detached]\n")
}

// makeRaw and restore are in terminal_darwin.go and terminal_linux.go

// drainStdin consumes any pending input from stdin in non-blocking mode
func drainStdin() {
	// Set stdin to non-blocking mode temporarily
	fd := int(os.Stdin.Fd())
	if err := syscall.SetNonblock(fd, true); err != nil {
		return
	}
	defer syscall.SetNonblock(fd, false)

	// Drain all available data
	buf := make([]byte, 1024)
	for {
		_, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}
	}
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
