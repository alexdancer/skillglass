package acceptance_test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type launchReceipt struct {
	ID            string   `json:"id"`
	Path          string   `json:"-"`
	Argv          []string `json:"argv"`
	Environment   []string `json:"environment"`
	Cwd           string   `json:"cwd"`
	Executable    string   `json:"executable"`
	Profile       string   `json:"profile,omitempty"`
	ProfileSHA256 string   `json:"profile_sha256,omitempty"`
	BinarySHA256  string   `json:"binary_sha256"`
	SystemInputs  string   `json:"system_inputs,omitempty"`
	ProductWrites string   `json:"product_writes,omitempty"`
	TmuxControl   string   `json:"tmux_control,omitempty"`
}

func recordLaunch(t *testing.T, command *exec.Cmd) *launchReceipt {
	t.Helper()
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	cwd := command.Dir
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
	}
	cwd = canonicalPath(t, cwd)
	resolve := func(path string) string {
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		return canonicalPath(t, path)
	}
	receipt := &launchReceipt{ID: hex.EncodeToString(nonce[:]), Argv: append([]string(nil), command.Args...), Environment: command.Environ(), Cwd: cwd, Executable: command.Path}
	if command.Path == "/usr/bin/sandbox-exec" && len(command.Args) >= 4 && (command.Args[1] == "-f" || command.Args[1] == "-p") {
		profile := []byte(command.Args[2])
		if command.Args[1] == "-f" {
			var err error
			profile, err = os.ReadFile(resolve(command.Args[2]))
			if err != nil {
				t.Fatal(err)
			}
		}
		digest := sha256.Sum256(profile)
		receipt.Profile, receipt.ProfileSHA256 = string(profile), hex.EncodeToString(digest[:])
		receipt.Executable = command.Args[3]
		if command.Args[1] == "-f" {
			receipt.SystemInputs = "controlled absence, ENOENT; no live contents inspected"
			receipt.ProductWrites = "denied"
			if strings.Contains(string(profile), "(deny file-write* (require-not (subpath ") && !strings.Contains(string(profile), "(deny file-write*)") {
				receipt.ProductWrites = "allowed only beneath the canonical disposable fixture root"
			}
			receipt.TmuxControl = "separate utility with explicit private socket; product payload has no network allowance"
		}
	}
	receipt.Executable = resolve(receipt.Executable)
	raw, err := os.ReadFile(receipt.Executable)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	receipt.BinarySHA256 = hex.EncodeToString(digest[:])
	receipt.Path = filepath.ToSlash(filepath.Join("fixture-policy", receipt.ID+".json"))
	encoded, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeExclusiveEvidence(t, receipt.Path, append(encoded, '\n'))
	return receipt
}

func launchExitCode(err error) int {
	if err == nil {
		return 0
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode()
	}
	return -1
}

func writeExclusiveEvidence(t *testing.T, relative string, data []byte) {
	t.Helper()
	root := os.Getenv("SKILLGLASS_EVIDENCE")
	if root == "" {
		return
	}
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}
