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
	if len(os.Args) == 9 && os.Args[1] == "boundary" {
		boundary()
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

func boundary() {
	writer := os.Args[2] == "true"
	connection, networkError := net.DialTimeout("tcp", "127.0.0.1:9", time.Second)
	if connection != nil {
		_ = connection.Close()
	}
	child := exec.Command("/usr/bin/true")
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	childError := child.Run()
	allowed, allowedError := os.ReadFile(os.Args[3])
	_, excludedError := os.ReadFile(os.Args[4])
	writeError := os.WriteFile(os.Args[5], []byte("fixture-write\n"), 0600)
	outsideWriteError := os.WriteFile(os.Args[6], []byte("forbidden\n"), 0600)
	_, configError := os.ReadFile(os.Args[7])
	_, skillsError := os.ReadDir(os.Args[8])
	// Absolute inherited paths are synthetic ENOENT, never host-content probes.
	_, inheritedConfigError := os.ReadFile("/etc/codex/config.toml")
	_, inheritedSkillsError := os.ReadDir("/etc/codex/skills")
	checks := map[string]bool{
		"allowed_read":                   allowedError == nil && string(allowed) == "ALLOWED_READ_SENTINEL\n",
		"excluded_read_denied":           os.IsPermission(excludedError),
		"network_denied":                 denied(networkError),
		"unapproved_child_exec_denied":   os.IsPermission(childError),
		"fixture_write_matches_mode":     (writer && writeError == nil) || (!writer && os.IsPermission(writeError)),
		"outside_write_denied":           os.IsPermission(outsideWriteError),
		"owned_system_config_absent":     os.IsNotExist(configError),
		"owned_system_skills_absent":     os.IsNotExist(skillsError),
		"inherited_system_config_absent": os.IsNotExist(inheritedConfigError),
		"inherited_system_skills_absent": os.IsNotExist(inheritedSkillsError),
	}
	passed := true
	for _, ok := range checks {
		passed = passed && ok
	}
	_ = json.NewEncoder(os.Stdout).Encode(result{Passed: passed, Checks: checks, Errors: map[string]string{"allowed": errorString(allowedError), "excluded": errorString(excludedError), "write": errorString(writeError), "outside_write": errorString(outsideWriteError), "config": errorString(configError), "skills": errorString(skillsError)}})
	if !passed {
		os.Exit(1)
	}
}
