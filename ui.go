package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func showMenu(hosts []SSHHost) {
	fmt.Print("\033[2J\033[H") // Clear screen
	fmt.Println("╔════════════════════════════════════════╗")
	fmt.Println("║    sshtui - Session Manager            ║")
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
		fwdInfo := displayForwards(host.Forwards)
		if fwdInfo != "" {
			fmt.Print(fwdInfo)
		}
		fmt.Println()
	}

	fmt.Println("\nCommands:")
	fmt.Println("  [number]  - Connect to host")
	fmt.Println("  [!number] - Resume session")
	fmt.Println("  v         - View scrollback/history")
	fmt.Println("  m         - Multi-host command")
	fmt.Println("  f         - Port forward info")
	fmt.Println("  x         - Close active session")
	fmt.Println("  q         - Quit all")
	fmt.Println("\nIn session: Ctrl+Space to detach")
	fmt.Print("\n> ")
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

func selectHosts(hosts []SSHHost) []SSHHost {
	reader := bufio.NewReader(os.Stdin)
	selected := make(map[int]bool)

	for {
		fmt.Print("\033[2J\033[H")
		fmt.Println("╔════════════════════════════════════════╗")
		fmt.Println("║ Select Hosts (space to toggle)        ║")
		fmt.Println("╚════════════════════════════════════════╝\n")

		for i, host := range hosts {
			marker := "[ ]"
			if selected[i] {
				marker = "[X]"
			}
			fmt.Printf("  %s [%d] %s", marker, i+1, host.Alias)
			if host.HostName != "" {
				fmt.Printf(" (%s)", host.HostName)
			}
			fmt.Println()
		}

		fmt.Println("\nCommands:")
		fmt.Println("  [number]  - Toggle selection")
		fmt.Println("  a         - Select all")
		fmt.Println("  c         - Clear all")
		fmt.Println("  d         - Done (execute)")
		fmt.Println("  q         - Cancel")
		fmt.Print("\n> ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		switch {
		case input == "q":
			return nil

		case input == "d":
			result := []SSHHost{}
			for i, host := range hosts {
				if selected[i] {
					result = append(result, host)
				}
			}
			return result

		case input == "a":
			for i := range hosts {
				selected[i] = true
			}

		case input == "c":
			selected = make(map[int]bool)

		default:
			var num int
			if _, err := fmt.Sscanf(input, "%d", &num); err == nil {
				if num > 0 && num <= len(hosts) {
					idx := num - 1
					selected[idx] = !selected[idx]
				}
			}
		}
	}
}
