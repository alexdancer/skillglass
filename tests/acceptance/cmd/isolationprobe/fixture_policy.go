package main

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func fixturePolicy() {
	if len(os.Args) != 8 && len(os.Args) != 9 {
		os.Exit(2)
	}
	allowed, allowedErr := os.ReadFile(os.Args[2])
	_, excludedErr := os.ReadFile(os.Args[3])
	_, statErr := os.Stat(os.Args[4])
	_, readErr := os.ReadFile(os.Args[4])
	_, dirErr := os.ReadDir(filepath.Dir(os.Args[4]))
	writeErr := os.WriteFile(os.Args[5], []byte("escape"), 0600)
	linkErr := os.WriteFile(os.Args[6], []byte("escape"), 0600)
	connection, networkErr := net.DialTimeout("tcp", os.Args[7], time.Second)
	if connection != nil {
		connection.Close()
	}
	checks := map[string]bool{"fixture_read": allowedErr == nil && string(allowed) == "ALLOWED_READ_SENTINEL\n", "excluded_read": os.IsPermission(excludedErr), "system_stat_absent": os.IsNotExist(statErr), "system_read_absent": os.IsNotExist(readErr), "system_directory_absent": os.IsNotExist(dirErr), "write_denied": os.IsPermission(writeErr), "symlink_write_denied": os.IsPermission(linkErr), "network_denied": denied(networkErr)}
	if len(os.Args) == 8 {
		binary, err := os.Executable()
		if err != nil {
			os.Exit(2)
		}
		command := exec.Command(binary, append(os.Args[1:], "--child")...)
		command.Stdin, command.Stderr = os.Stdin, os.Stderr
		child, err := command.Output()
		var nested result
		if err == nil && json.Unmarshal(child, &nested) == nil {
			for name, passed := range nested.Checks {
				checks["inherited_"+name] = passed
			}
		} else {
			checks["inherited_child"] = false
		}
	}
	passed := true
	for _, value := range checks {
		passed = passed && value
	}
	json.NewEncoder(os.Stdout).Encode(result{Passed: passed, Checks: checks})
	if !passed {
		os.Exit(1)
	}
}
