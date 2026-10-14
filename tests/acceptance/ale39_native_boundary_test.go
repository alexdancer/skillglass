package acceptance_test

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestALE39NativeHelperVersionContainment(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Fatal("Darwin sandbox required; no containment bypass")
	}
	artifacts := t.TempDir()
	base, err := os.MkdirTemp("/private/tmp", "ale39-inert-boundary-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(base)
	canary, child, outsideWrite := filepath.Join(base, "canary.txt"), filepath.Join(base, "child"), filepath.Join(base, "outside-write.txt")
	writeFile(t, canary, []byte("CONTROLLED_OUTSIDE_CANARY\n"), 0600)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	template, err := os.ReadFile(filepath.Join(repositoryRoot, "tests/native_verification/fixtures/version.go.txt"))
	if err != nil {
		t.Fatal(err)
	}
	fixtureSource := strings.NewReplacer("@CANARY@", canary, "@CHILD@", child, "@WRITE@", outsideWrite, "@ADDRESS@", listener.Addr().String()).Replace(string(template))
	fixtureMain := filepath.Join(base, "main.go")
	writeFile(t, fixtureMain, []byte(fixtureSource), 0600)
	binary := filepath.Join(base, "inert-version")
	build := exec.Command("/opt/homebrew/bin/go", "build", "-o", binary, fixtureMain)
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOPROXY=off", "GOSUMDB=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("inert fixture build: %v %s", err, out)
	}
	bytes, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, child, bytes, 0700)
	// This repository stand-in only satisfies the helper's initial revision check.
	// No native source is supplied, copied, compiled, executed, or claimed validated.
	source := filepath.Join(base, "source")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	init := exec.Command("/usr/bin/git", "init", "-q", source)
	init.Env = []string{"HOME=" + base, "PATH=/usr/bin:/bin", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
	if out, err := init.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	writeFile(t, filepath.Join(source, ".git/HEAD"), []byte("7e30335645c8c792849de1c13b3547d75775baa4\n"), 0600)
	controlCase := filepath.Join(base, "control")
	controlProject := filepath.Join(controlCase, "project")
	if err := os.MkdirAll(filepath.Join(controlCase, "tmp"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(controlProject, 0700); err != nil {
		t.Fatal(err)
	}
	observed := map[string]any{"fixture_base": base, "fixture_binary": binary, "canary": canary, "source_standin": source}
	invoke := func(name string, argv []string, cwd string) map[string]any {
		t.Helper()
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = cwd
		stdout, err := cmd.Output()
		exit := 0
		stderr := ""
		if failure, ok := err.(*exec.ExitError); ok {
			exit = failure.ExitCode()
			stderr = string(failure.Stderr)
		} else if err != nil {
			t.Fatal(err)
		}
		receipt := map[string]any{"argv": argv, "cwd": cwd, "exit": exit, "stdout": string(stdout), "stderr": stderr}
		observed[name] = receipt
		if exit != 73 {
			t.Fatalf("%s fixture did not reach deliberate exit 73: %+v", name, receipt)
		}
		var probe map[string]any
		if err := json.Unmarshal(stdout, &probe); err != nil {
			t.Fatalf("%s output: %v %s", name, err, stdout)
		}
		return probe
	}
	positive := invoke("positive-control", []string{binary, "version"}, controlProject)
	if positive["outside_content_read"] != true || positive["child_executed"] != true || positive["same_executable_child"] != true || positive["outside_write_denied"] != false || positive["network_denied"] != false {
		t.Fatalf("positive controls not capable: %+v", positive)
	}
	if err := os.Remove(outsideWrite); err != nil {
		t.Fatal(err)
	}
	profile := fmt.Sprintf(`(version 1) (allow default) (deny network*) (deny file-write*) (allow file-write* (subpath %q)) (deny file-read* (literal %q)) (deny process-exec (literal %q))`, controlCase, canary, child)
	strict := invoke("targeted-denial-control", []string{"/usr/bin/sandbox-exec", "-p", profile, binary, "version"}, controlProject)
	if strict["outside_content_read"] != false || strict["child_executed"] != false || strict["inside_write_allowed"] != true || strict["outside_write_denied"] != true || strict["network_denied"] != true {
		t.Fatalf("targeted denial controls failed: %+v", strict)
	}
	argv := []string{"/usr/bin/python3", filepath.Join(repositoryRoot, "tools/verify_codeaf.py"), "--source", source, "--binary", binary, "--output", filepath.Join(base, "output")}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = repositoryRoot
	stdout, err := cmd.Output()
	exit := 0
	stderr := ""
	if failure, ok := err.(*exec.ExitError); ok {
		exit = failure.ExitCode()
		stderr = string(failure.Stderr)
	} else if err != nil {
		t.Fatal(err)
	}
	observed["public-cli"] = map[string]any{"argv": argv, "cwd": repositoryRoot, "exit": exit, "stdout": string(stdout), "stderr": stderr}
	if exit != 1 {
		t.Fatalf("helper should stop at inert exit 73, got %d: %s", exit, stderr)
	}
	var receipt struct {
		Exit   int    `json:"exit"`
		Stdout string `json:"stdout"`
	}
	index := strings.LastIndex(stderr, "RuntimeError: ")
	if index < 0 {
		t.Fatalf("no fixture receipt: %s", stderr)
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr[index+len("RuntimeError: "):])), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Exit != 73 {
		t.Fatalf("public CLI did not reach fixture version: %s", stderr)
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(receipt.Stdout), &probe); err != nil {
		t.Fatal(err)
	}
	observed["public-cli-probe"] = probe
	for _, name := range []string{"receipts.json", "containment.json"} {
		data, err := os.ReadFile(filepath.Join(base, "output", name))
		if err != nil {
			t.Fatal(err)
		}
		writeEvidence(t, "native-helper/"+name, data)
	}
	raw, err := json.MarshalIndent(observed, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeEvidence(t, "native-helper/boundary-receipts.json", raw)
	if err := os.WriteFile(filepath.Join(artifacts, "boundary-receipts.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if probe["inside_write_allowed"] != true || probe["outside_write_denied"] != true || probe["network_denied"] != true {
		t.Fatalf("helper's documented write/network restrictions did not hold: %+v", probe)
	}
	if probe["outside_content_read"] != false {
		t.Errorf("literal containment requirement: outside_content_read = %v, want false", probe["outside_content_read"])
	}
	if probe["same_executable_child"] != false {
		t.Errorf("same executable child = %v, want false", probe["same_executable_child"])
	}
	for _, key := range []string{"outside_stat_denied", "outside_list_denied", "symlink_read_denied", "fixture_read_allowed"} {
		if probe[key] != true {
			t.Errorf("%s = %v, want true", key, probe[key])
		}
	}
	t.Logf("public helper probe: %s", receipt.Stdout)
	if probe["child_executed"] != false {
		t.Errorf("literal version-only requirement: child_executed = %v, want false", probe["child_executed"])
	}
	// An outer restriction makes the helper's containment bootstrap unavailable.
	// This must stop before supplied execution, with no fallback to the host.
	blockedOutput := filepath.Join(base, "blocked-output")
	blocked := exec.Command("/usr/bin/sandbox-exec", "-p", "(version 1) (allow default) (deny network*)", argv[0], argv[1], "--source", source, "--binary", binary, "--output", blockedOutput)
	blocked.Dir = repositoryRoot
	blockedLog, blockedErr := blocked.CombinedOutput()
	if blockedErr == nil {
		t.Fatal("unavailable containment unexpectedly succeeded")
	}
	writeEvidence(t, "native-helper/blocked.log", blockedLog)
	blockedReceipts, err := os.ReadFile(filepath.Join(blockedOutput, "receipts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var commands []struct {
		Argv []string `json:"argv"`
	}
	if err := json.Unmarshal(blockedReceipts, &commands); err != nil {
		t.Fatal(err)
	}
	for _, receipt := range commands {
		if len(receipt.Argv) > 0 && receipt.Argv[len(receipt.Argv)-1] == "version" {
			t.Fatalf("supplied version launched after failed containment: %+v", receipt)
		}
	}
	if _, err := os.Stat(filepath.Join(blockedOutput, "containment.json")); !os.IsNotExist(err) {
		t.Fatalf("failed containment published proof: %v", err)
	}
	writeEvidence(t, "native-helper/blocked-receipts.json", blockedReceipts)
}
