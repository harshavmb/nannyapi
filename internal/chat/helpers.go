package chat

import "strings"

func SendHealthCheckCommands() []string {
	// Send health check commands to agent
	// from https://netflixtechblog.com/linux-performance-analysis-in-60-000-milliseconds-accc10403c55
	allowedCommands := []AllowedCommand{
		{Command: "uptime", Args: []string{}},                              // system uptime
		{Command: "dmesg", Args: []string{"|", "tail"}},                    // for oom-killer, and TCP dropping a request
		{Command: "vmstat", Args: []string{"1", "5"}},                      // virtual memory statistics
		{Command: "iostat", Args: []string{"-xz", "1", "5"}},               // I/O statistics
		{Command: "mpstat", Args: []string{"-P", "ALL", "1", "5"}},         // CPU statistics
		{Command: "pidstat", Args: []string{"1", "5"}},                     // process statistics
		{Command: "free", Args: []string{"-m"}},                            // memory statistics
		{Command: "sar", Args: []string{"-n", "DEV", "1", "5"}},            // network statistics
		{Command: "sar", Args: []string{"-n", "TCP,ETCP", "1", "5"}},       // TCP statistics
		{Command: "sar", Args: []string{"-n", "SOCK", "1", "5"}},           // socket statistics
		{Command: "top", Args: []string{"-b", "-n", "5", "-d", "1", "-c"}}, // top processes
	}
	// Convert the allowed commands to a list of strings
	var commands []string
	for _, cmd := range allowedCommands {
		commandStr := cmd.Command
		if len(cmd.Args) > 0 {
			commandStr += " " + strings.Join(cmd.Args, " ")
		}
		commands = append(commands, commandStr)
	}
	return commands
}
