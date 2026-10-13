package main

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type result struct {
	Passed bool              `json:"passed"`
	Checks map[string]bool   `json:"checks"`
	Errors map[string]string `json:"errors"`
}

func main() {
	if len(os.Args) == 3 && os.Args[1] == "--system" {
		_, readErr := os.ReadFile(os.Args[2])
		_, statErr := os.Stat(os.Args[2])
		_, dirErr := os.ReadDir(filepath.Dir(os.Args[2]))
		passed := os.IsNotExist(readErr) && os.IsNotExist(statErr) && os.IsNotExist(dirErr)
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"passed": passed, "read_absent": os.IsNotExist(readErr), "stat_absent": os.IsNotExist(statErr), "directory_absent": os.IsNotExist(dirErr)})
		if !passed {
			os.Exit(1)
		}
		return
	}
	if len(os.Args) != 5 {
		os.Exit(2)
	}
	allowed, allowedError := os.ReadFile(os.Args[1])
	_, excludedError := os.ReadFile(os.Args[2])
	writeError := os.WriteFile(os.Args[3], []byte("forbidden\n"), 0o600)
	connection, networkError := net.DialTimeout("tcp", os.Args[4], time.Second)
	if connection != nil {
		_ = connection.Close()
	}
	child := exec.Command("/usr/bin/true")
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	childError := child.Run()

	checks := map[string]bool{
		"allowed_read":         allowedError == nil && string(allowed) == "ALLOWED_READ_SENTINEL\n",
		"excluded_read_denied": os.IsPermission(excludedError),
		"file_write_denied":    os.IsPermission(writeError),
		"network_denied":       denied(networkError),
		"child_exec_denied":    os.IsPermission(childError),
	}
	passed := true
	for _, check := range checks {
		passed = passed && check
	}
	output := result{
		Passed: passed,
		Checks: checks,
		Errors: map[string]string{
			"allowed":  errorString(allowedError),
			"excluded": errorString(excludedError),
			"write":    errorString(writeError),
			"network":  errorString(networkError),
			"child":    errorString(childError),
		},
	}
	_ = json.NewEncoder(os.Stdout).Encode(output)
	if !passed {
		os.Exit(1)
	}
}

func denied(err error) bool {
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
