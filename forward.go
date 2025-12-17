package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func manageForwards(hosts []SSHHost) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("\033[2J\033[H")
		fmt.Println("╔════════════════════════════════════════╗")
		fmt.Println("║ Port Forward Management                ║")
		fmt.Println("╚════════════════════════════════════════╝\n")

		fmt.Println("Configured Forwards:")
		hasForwards := false
		for _, host := range hosts {
			if len(host.Forwards) > 0 {
				hasForwards = true
				fmt.Printf("\n  %s:\n", host.Alias)
				for _, fwd := range host.Forwards {
					switch fwd.Type {
					case "L":
						fmt.Printf("    Local:   %s → %s\n", fwd.LocalPort, fwd.RemoteAddr)
					case "R":
						fmt.Printf("    Remote:  %s → %s\n", fwd.LocalPort, fwd.RemoteAddr)
					case "D":
						fmt.Printf("    Dynamic: %s (SOCKS)\n", fwd.LocalPort)
					}
				}
			}
		}

		if !hasForwards {
			fmt.Println("  No port forwards configured")
		}

		fmt.Println("\n\nActive Session Forwards:")
		hasActiveForwards := false
		sessionsMu.RLock()
		for _, session := range sessions {
			// Find the host for this session
			for _, host := range hosts {
				if host.Alias == session.Alias && len(host.Forwards) > 0 {
					hasActiveForwards = true
					status := "alive"
					if session.Cmd.ProcessState != nil && session.Cmd.ProcessState.Exited() {
						status = "ended"
					}
					fmt.Printf("\n  Session [!%d] %s (%s):\n", session.ID, session.Alias, status)
					for _, fwd := range host.Forwards {
						switch fwd.Type {
						case "L":
							fmt.Printf("    L: %s → %s\n", fwd.LocalPort, fwd.RemoteAddr)
						case "R":
							fmt.Printf("    R: %s → %s\n", fwd.LocalPort, fwd.RemoteAddr)
						case "D":
							fmt.Printf("    D: %s\n", fwd.LocalPort)
						}
					}
					break
				}
			}
		}
		sessionsMu.RUnlock()

		if !hasActiveForwards {
			fmt.Println("  No active forwards")
		}

		fmt.Println("\n\nNote: Port forwards are configured in ~/.ssh/config")
		fmt.Println("Format:")
		fmt.Println("  LocalForward 8080 remote:80")
		fmt.Println("  RemoteForward 9090 localhost:80")
		fmt.Println("  DynamicForward 1080")

		fmt.Println("\nCommands:")
		fmt.Println("  q - Back to main menu")
		fmt.Print("\n> ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "q" {
			return
		}
	}
}
