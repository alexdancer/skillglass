package main

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
)

type result struct {
	Passed bool              `json:"passed"`
	Checks map[string]bool   `json:"checks"`
	Errors map[string]string `json:"errors"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "fixture-policy" {
		fixturePolicy()
		return
	}
	if len(os.Args) != 5 && len(os.Args) != 6 {
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
	if len(os.Args) == 6 {
		_, statError := os.Stat(os.Args[2])
		_, listingError := os.ReadDir(os.Args[5])
		_, configStatError := os.Stat("/etc/codex/config.toml")
		_, configReadError := os.ReadFile("/etc/codex/config.toml")
		_, systemStatError := os.Stat("/etc/codex/skills")
		_, systemListingError := os.ReadDir("/etc/codex/skills")
		checks["excluded_read_denied"] = os.IsNotExist(excludedError)
		checks["model_stat_absent"] = os.IsNotExist(statError)
		checks["model_listing_absent"] = os.IsNotExist(listingError)
		checks["system_config_absent"] = os.IsNotExist(configStatError) && os.IsNotExist(configReadError)
		checks["system_skills_absent"] = os.IsNotExist(systemStatError) && os.IsNotExist(systemListingError)
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
