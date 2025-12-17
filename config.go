package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SSHHost represents a parsed SSH host
type SSHHost struct {
	Alias    string
	HostName string
	User     string
	Port     string
	Forwards []PortForward
}

// PortForward represents an SSH port forward
type PortForward struct {
	Type       string // "L", "R", "D"
	LocalPort  string
	RemoteAddr string // "host:port" or empty for dynamic
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

			current = &SSHHost{
				Alias:    value,
				Forwards: make([]PortForward, 0),
			}
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
		case "localforward":
			fwd := parseLocalForward(value)
			if fwd != nil {
				current.Forwards = append(current.Forwards, *fwd)
			}
		case "remoteforward":
			fwd := parseRemoteForward(value)
			if fwd != nil {
				current.Forwards = append(current.Forwards, *fwd)
			}
		case "dynamicforward":
			fwd := parseDynamicForward(value)
			if fwd != nil {
				current.Forwards = append(current.Forwards, *fwd)
			}
		}
	}

	if current != nil {
		hosts = append(hosts, *current)
	}

	return hosts, scanner.Err()
}

func parseLocalForward(value string) *PortForward {
	// LocalForward 8080 remote:80
	parts := strings.Fields(value)
	if len(parts) < 2 {
		return nil
	}

	return &PortForward{
		Type:       "L",
		LocalPort:  parts[0],
		RemoteAddr: parts[1],
	}
}

func parseRemoteForward(value string) *PortForward {
	// RemoteForward 9090 localhost:80
	parts := strings.Fields(value)
	if len(parts) < 2 {
		return nil
	}

	return &PortForward{
		Type:       "R",
		LocalPort:  parts[0],
		RemoteAddr: parts[1],
	}
}

func parseDynamicForward(value string) *PortForward {
	// DynamicForward 1080
	port := strings.TrimSpace(value)
	if port == "" {
		return nil
	}

	return &PortForward{
		Type:      "D",
		LocalPort: port,
	}
}

func buildSSHArgs(host SSHHost) []string {
	args := []string{}

	// Add port forwards
	for _, fwd := range host.Forwards {
		switch fwd.Type {
		case "L":
			args = append(args, "-L", fmt.Sprintf("%s:%s", fwd.LocalPort, fwd.RemoteAddr))
		case "R":
			args = append(args, "-R", fmt.Sprintf("%s:%s", fwd.LocalPort, fwd.RemoteAddr))
		case "D":
			args = append(args, "-D", fwd.LocalPort)
		}
	}

	args = append(args, host.Alias)
	return args
}

func displayForwards(forwards []PortForward) string {
	if len(forwards) == 0 {
		return ""
	}

	parts := []string{}
	for _, fwd := range forwards {
		switch fwd.Type {
		case "L":
			parts = append(parts, fmt.Sprintf("L:%s→%s", fwd.LocalPort, fwd.RemoteAddr))
		case "R":
			parts = append(parts, fmt.Sprintf("R:%s→%s", fwd.LocalPort, fwd.RemoteAddr))
		case "D":
			parts = append(parts, fmt.Sprintf("D:%s", fwd.LocalPort))
		}
	}

	return " [" + strings.Join(parts, ", ") + "]"
}
