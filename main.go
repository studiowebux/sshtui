package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const version = "0.0.2"

func main() {
	// Handle CLI flags
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if arg == "--version" || arg == "-v" {
			fmt.Printf("sshtui v%s\n", version)
			os.Exit(0)
		}
		if arg == "--help" || arg == "-h" {
			fmt.Println("sshtui - SSH session manager")
			fmt.Printf("Version: %s\n\n", version)
			fmt.Println("Usage: sshtui [options]")
			fmt.Println("\nOptions:")
			fmt.Println("  -v, --version    Show version")
			fmt.Println("  -h, --help       Show help")
			os.Exit(0)
		}
	}

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
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError reading input: %v\n", err)
			closeAllSessions()
			break
		}
		input = strings.TrimSpace(input)

		if input == "q" {
			closeAllSessions()
			break
		}

		if input == "x" {
			closeActiveSession()
			continue
		}

		if input == "v" {
			// View scrollback
			sessionsMu.RLock()
			hasSession := len(sessions) > 0
			sessionsMu.RUnlock()

			if hasSession {
				fmt.Print("Which session? [!number]: ")
				reader := bufio.NewReader(os.Stdin)
				numStr, err := reader.ReadString('\n')
				if err != nil {
					fmt.Printf("Error reading input: %v\n", err)
					continue
				}
				numStr = strings.TrimSpace(numStr)
				var num int
				if strings.HasPrefix(numStr, "!") {
					if _, err := fmt.Sscanf(numStr, "!%d", &num); err == nil {
						sessionsMu.RLock()
						if num > 0 && num <= len(sessions) {
							session := sessions[num-1]
							sessionsMu.RUnlock()
							viewScrollback(session)
						} else {
							sessionsMu.RUnlock()
							fmt.Println("Invalid session number")
						}
					}
				}
			} else {
				fmt.Println("No active sessions")
			}
			continue
		}

		if input == "m" {
			// Multi-host command execution
			selectedHosts := selectHosts(hosts)
			if selectedHosts != nil {
				executeMultiHost(selectedHosts)
			}
			continue
		}

		if input == "f" {
			// Port forward management
			manageForwards(hosts)
			continue
		}

		// Check for session (!number) or host (number)
		if strings.HasPrefix(input, "!") {
			// Resume session
			var num int
			if _, err := fmt.Sscanf(input, "!%d", &num); err == nil {
				sessionsMu.RLock()
				if num > 0 && num <= len(sessions) {
					session := sessions[num-1]
					sessionsMu.RUnlock()
					attachToSession(session)
				} else {
					sessionsMu.RUnlock()
					fmt.Printf("Invalid session number: %d (have %d sessions)\n", num, len(sessions))
					fmt.Println("Press Enter to continue...")
					bufio.NewReader(os.Stdin).ReadString('\n')
				}
			} else {
				fmt.Printf("Invalid format: %s (expected !number)\n", input)
				fmt.Println("Press Enter to continue...")
				bufio.NewReader(os.Stdin).ReadString('\n')
			}
			continue
		}

		// Connect to new host
		var num int
		if _, err := fmt.Sscanf(input, "%d", &num); err == nil {
			if num > 0 && num <= len(hosts) {
				createSession(hosts[num-1])
			} else {
				fmt.Println("Invalid host number")
			}
		} else if input != "" {
			fmt.Println("Invalid command. Press Enter to continue...")
			bufio.NewReader(os.Stdin).ReadString('\n')
		}
	}
}
