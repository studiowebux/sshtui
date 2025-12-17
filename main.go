package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const version = "0.0.1"

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
		input, _ := reader.ReadString('\n')
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
