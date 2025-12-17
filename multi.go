package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/creack/pty"
)

type HostResult struct {
	Alias  string
	Output string
	Error  error
}

func executeMultiHost(hosts []SSHHost) {
	if len(hosts) == 0 {
		fmt.Println("No hosts selected. Press Enter...")
		bufio.NewReader(os.Stdin).ReadString('\n')
		return
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nEnter command to execute: ")
	command, _ := reader.ReadString('\n')
	command = strings.TrimSpace(command)

	if command == "" {
		return
	}

	fmt.Print("\nDisplay mode:\n")
	fmt.Println("  [1] Live streaming (see output as it arrives)")
	fmt.Println("  [2] Collected results (all at once)")
	fmt.Print("> ")

	modeInput, _ := reader.ReadString('\n')
	modeInput = strings.TrimSpace(modeInput)

	if modeInput == "1" {
		executeMultiHostLive(hosts, command)
	} else {
		executeMultiHostCollected(hosts, command)
	}
}

func executeMultiHostLive(hosts []SSHHost, command string) {
	fmt.Print("\033[2J\033[H")
	fmt.Println("╔════════════════════════════════════════╗")
	fmt.Println("║ Multi-Host Execution (Live)            ║")
	fmt.Println("╚════════════════════════════════════════╝\n")
	fmt.Printf("Command: %s\n\n", command)

	var wg sync.WaitGroup
	outputMutex := sync.Mutex{}

	for _, host := range hosts {
		wg.Add(1)
		go func(h SSHHost) {
			defer wg.Done()

			args := buildSSHArgs(h)
			args = append(args, command)
			cmd := exec.Command("ssh", args...)

			// Use PTY to handle passphrase prompts
			ptmx, err := pty.Start(cmd)
			if err != nil {
				outputMutex.Lock()
				fmt.Printf("─────────────────────────────────────────\n")
				fmt.Printf("Host: %s\n", h.Alias)
				fmt.Printf("Error starting: %v\n", err)
				outputMutex.Unlock()
				return
			}
			defer ptmx.Close()

			// Copy stdin to PTY for passphrase (if needed)
			go io.Copy(ptmx, os.Stdin)

			// Collect output
			var output bytes.Buffer
			io.Copy(&output, ptmx)

			cmd.Wait()

			outputMutex.Lock()
			defer outputMutex.Unlock()

			fmt.Printf("─────────────────────────────────────────\n")
			fmt.Printf("Host: %s\n", h.Alias)
			fmt.Printf("\n%s\n", output.String())
		}(host)
	}

	wg.Wait()

	fmt.Println("─────────────────────────────────────────")
	fmt.Println("\nExecution complete. Press Enter...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}

func executeMultiHostCollected(hosts []SSHHost, command string) {
	fmt.Print("\033[2J\033[H")
	fmt.Println("╔════════════════════════════════════════╗")
	fmt.Println("║ Multi-Host Execution (Collecting...)   ║")
	fmt.Println("╚════════════════════════════════════════╝\n")

	results := make([]HostResult, len(hosts))
	var wg sync.WaitGroup

	for i, host := range hosts {
		wg.Add(1)
		go func(idx int, h SSHHost) {
			defer wg.Done()

			args := buildSSHArgs(h)
			args = append(args, command)
			cmd := exec.Command("ssh", args...)

			// Use PTY to handle passphrase prompts
			ptmx, err := pty.Start(cmd)
			if err != nil {
				results[idx] = HostResult{
					Alias:  h.Alias,
					Output: "",
					Error:  err,
				}
				return
			}
			defer ptmx.Close()

			// Copy stdin to PTY for passphrase (if needed)
			go io.Copy(ptmx, os.Stdin)

			// Collect output
			var output bytes.Buffer
			io.Copy(&output, ptmx)

			cmd.Wait()

			results[idx] = HostResult{
				Alias:  h.Alias,
				Output: output.String(),
				Error:  nil,
			}

			fmt.Printf("  ✓ %s\n", h.Alias)
		}(i, host)
	}

	wg.Wait()

	// Display results
	fmt.Print("\033[2J\033[H")
	fmt.Println("╔════════════════════════════════════════╗")
	fmt.Println("║ Multi-Host Results                     ║")
	fmt.Println("╚════════════════════════════════════════╝\n")
	fmt.Printf("Command: %s\n\n", command)

	for _, result := range results {
		fmt.Printf("─────────────────────────────────────────\n")
		fmt.Printf("Host: %s\n", result.Alias)
		if result.Error != nil {
			fmt.Printf("Error: %v\n", result.Error)
		}
		fmt.Printf("\n%s\n", result.Output)
	}

	fmt.Println("─────────────────────────────────────────")
	fmt.Println("\nPress Enter...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
