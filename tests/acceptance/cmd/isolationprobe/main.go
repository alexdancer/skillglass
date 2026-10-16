package main

import (
	"encoding/json"
	"errors"
	"io"
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
	if len(os.Args) == 4 && os.Args[1] == "--network-positive" {
		networkErr, sent, writeErr := probeNetwork(os.Args[2], os.Args[3])
		passed := networkErr == nil && writeErr == nil && sent == len(os.Args[3])
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"passed": passed, "sent": sent, "network_error": errorString(networkErr), "write_error": errorString(writeErr)})
		if !passed {
			os.Exit(1)
		}
		return
	}
	networkCanary := ""
	if len(os.Args) > 3 && os.Args[1] == "--network-canary" {
		networkCanary = os.Args[2]
		os.Args = append(os.Args[:1], os.Args[3:]...)
	}
	if len(os.Args) == 9 && os.Args[1] == "boundary" {
		boundary()
		return
	}
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
	if len(os.Args) > 1 && os.Args[1] == "fixture-policy" {
		fixturePolicy()
		return
	}
	systemPolicy := len(os.Args) == 7 && os.Args[1] == "--system-policy"
	if systemPolicy {
		os.Args = append(os.Args[:1], os.Args[2:]...)
	}
	absence := len(os.Args) == 6 && os.Args[1] == "--absence"
	if absence {
		os.Args = append(os.Args[:1], os.Args[2:]...)
	}
	if len(os.Args) != 5 && len(os.Args) != 6 {
		os.Exit(2)
	}
	allowed, allowedError := os.ReadFile(os.Args[1])
	_, excludedError := os.ReadFile(os.Args[2])
	writeError := os.WriteFile(os.Args[3], []byte("forbidden\n"), 0o600)
	networkError, sent, networkWriteError := probeNetwork(os.Args[4], networkCanary)
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
	if networkCanary != "" {
		checks["network_canary_not_sent"] = sent == 0 && networkWriteError == nil
	}
	if absence {
		_, metadataError := os.Stat(os.Args[2])
		checks["excluded_read_denied"] = os.IsNotExist(excludedError)
		checks["excluded_metadata_absent"] = os.IsNotExist(metadataError)
	} else if systemPolicy {
		_, systemError := os.ReadFile(os.Args[5])
		checks["system_absent"] = os.IsNotExist(systemError)
	} else if len(os.Args) == 6 {
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

// probeNetwork is shared by the allowed child control and contained launch probes.
func probeNetwork(address, payload string) (error, int, error) {
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		return err, 0, nil
	}
	defer connection.Close()
	if payload == "" {
		return nil, 0, nil
	}
	if err := connection.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		return nil, 0, err
	}
	sent, writeErr := io.WriteString(connection, payload)
	return nil, sent, writeErr
}
