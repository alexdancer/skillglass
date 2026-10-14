package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDecisionFinalizedReceiptContext(t *testing.T) {
	if os.Getenv("SKILLGLASS_EVIDENCE") == "" {
		t.Setenv("SKILLGLASS_EVIDENCE", t.TempDir())
	}
	fx := newFixture(t)
	allowed := filepath.Join(fx.base, "allowed", "positive")
	excluded := filepath.Join(fx.base, "excluded", "outside")
	system := filepath.Join(fx.base, "excluded", "system-inputs", "settings.json")
	outside := filepath.Join(fx.base, "excluded", "escape")
	link := filepath.Join(fx.base, "allowed", "escape-link")
	writeFile(t, allowed, []byte("ALLOWED_READ_SENTINEL\n"), 0600)
	writeFile(t, excluded, []byte("inert excluded"), 0600)
	writeFile(t, system, []byte("inert system model"), 0600)
	if err := os.Symlink(filepath.Dir(outside), link); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{allowed, excluded, system} {
		if _, err := os.ReadFile(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(outside, []byte("positive"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(outside); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	connection, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
	if output, err := exec.Command("/bin/echo", "UNAPPROVED_CHILD_POSITIVE").Output(); err != nil || string(output) != "UNAPPROVED_CHILD_POSITIVE\n" {
		t.Fatal("positive exec", err, string(output))
	}

	run := func(label string, cmd *exec.Cmd) commandResult {
		profile, err := os.ReadFile(fx.policy)
		if err != nil {
			t.Fatal(err)
		}
		executable := cmd.Args[3]
		raw, err := os.ReadFile(executable)
		if err != nil {
			t.Fatal(err)
		}
		p, b := sha256.Sum256(profile), sha256.Sum256(raw)
		actual, _ := json.MarshalIndent(map[string]any{"argv": cmd.Args, "environment": cmd.Env, "cwd": cmd.Dir, "profile": string(profile), "profile_sha256": hex.EncodeToString(p[:]), "binary_sha256": hex.EncodeToString(b[:]), "note": "investigator finalized launch evidence, not original candidate receipt"}, "", "  ")
		writeEvidence(t, "investigator/"+label+"-launch.json", append(actual, '\n'))
		var out, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		started := time.Now()
		launch := recordLaunch(t, cmd)
		err = cmd.Run()
		result := commandResult{stdout: out.Bytes(), stderr: stderr.Bytes(), duration: time.Since(started), launch: launch}
		if err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				result.code = e.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		recordEvidenceCommand(t, "investigator-"+label, result)
		recordedBytes, err := os.ReadFile(filepath.Join(os.Getenv("SKILLGLASS_EVIDENCE"), launch.Path))
		if err != nil {
			t.Fatal(err)
		}
		var recorded launchReceipt
		decodeJSON(t, recordedBytes, &recorded)
		if recorded.ProfileSHA256 != hex.EncodeToString(p[:]) || recorded.BinarySHA256 != hex.EncodeToString(b[:]) || recorded.Profile != string(profile) || !reflect.DeepEqual(recorded.Argv, cmd.Args) {
			t.Error("launch hashes or finalized argv differ from actual inputs")
		}
		outputRecord, err := os.ReadFile(filepath.Join(os.Getenv("SKILLGLASS_EVIDENCE"), "commands", launch.ID, "case.json"))
		if err != nil {
			t.Fatal(err)
		}
		var linked evidenceCase
		decodeJSON(t, outputRecord, &linked)
		if linked.LaunchReceipt != launch.Path || !reflect.DeepEqual(linked.Argv, cmd.Args) || linked.ExitCode != result.code {
			t.Error("output/exit record lost immutable launch link")
		}
		return result
	}
	command := func(exe string, args ...string) *exec.Cmd {
		argv := fx.invocation(t, exe, args...)
		cmd := exec.CommandContext(context.Background(), argv[0], argv[1:]...)
		cmd.Dir, cmd.Env = fx.project, fx.environment()
		return cmd
	}
	controls := run("canaries", command(isolationProbeBinary, "fixture-policy", allowed, excluded, system, outside, filepath.Join(link, "escape"), listener.Addr().String()))
	assertExit(t, controls, 0)
	var checks struct {
		Passed bool
		Checks map[string]bool
	}
	decodeJSON(t, controls.stdout, &checks)
	if !checks.Passed || len(checks.Checks) != 16 {
		t.Fatalf("boundary failed: %s", controls.stdout)
	}
	unapproved := run("unapproved-child", command("/bin/echo", "UNAPPROVED_CHILD_NEGATIVE"))
	if unapproved.code == 0 || len(unapproved.stdout) != 0 {
		t.Fatal("unapproved child reached execution")
	}
	writeEvidence(t, "investigator/boundary.json", []byte("{\"controller_positive_controls\":true,\"all_16_direct_and_inherited_canaries\":true,\"unapproved_exec_denied\":true}\n"))

	defaultManifest := claudeSkill(t, filepath.Join(fx.home, ".claude", "skills"), "default", "default-home-literal", "")
	configured := filepath.Join(fx.base, "allowed", "configured-claude")
	configuredManifest := claudeSkill(t, filepath.Join(configured, "skills"), "configured", "configured-home-literal", "")
	actualCwd := filepath.Join(fx.base, "allowed", "actual-cwd")
	if err := os.MkdirAll(actualCwd, 0700); err != nil {
		t.Fatal(err)
	}
	before := treeManifest(t, fx.base)
	args := []string{"list", "--agent", "claude-code", "--project", fx.project, "--json"}
	configuredCmd := fx.command(t, context.Background(), args...)
	configuredCmd.Env = append(fx.environment(), "CLAUDE_CONFIG_DIR="+configured)
	configuredCmd.Dir = actualCwd
	first := run("configured", configuredCmd)
	assertExit(t, first, 0)
	var list codexList
	decodeJSON(t, first.stdout, &list)
	if claudeAt(t, list, filepath.Dir(configuredManifest)).Availability.State != "available" {
		t.Fatal("configured public result wrong")
	}
	for _, source := range list.Sources {
		if source.Name == "default-home-literal" {
			t.Fatal("configured context fell back")
		}
	}
	receiptPath := filepath.Join(os.Getenv("SKILLGLASS_EVIDENCE"), first.launch.Path)
	firstReceipt, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	writeEvidence(t, "investigator/configured-original-receipt.json", firstReceipt)
	var receipt struct {
		Environment []string `json:"environment"`
		Cwd         string   `json:"cwd"`
	}
	decodeJSON(t, firstReceipt, &receipt)
	environmentCorrect := reflect.DeepEqual(receipt.Environment, configuredCmd.Env)
	cwdCorrect := receipt.Cwd == configuredCmd.Dir

	secondCmd := fx.command(t, context.Background(), args...)
	second := run("default-control", secondCmd)
	assertExit(t, second, 0)
	decodeJSON(t, second.stdout, &list)
	if claudeAt(t, list, filepath.Dir(defaultManifest)).Availability.State != "available" {
		t.Fatal("default public control wrong")
	}
	for _, source := range list.Sources {
		if source.Name == "configured-home-literal" {
			t.Fatal("default context retained configured root")
		}
	}
	secondReceipt, err := os.ReadFile(filepath.Join(os.Getenv("SKILLGLASS_EVIDENCE"), second.launch.Path))
	if err != nil {
		t.Fatal(err)
	}
	writeEvidence(t, "investigator/default-original-receipt.json", secondReceipt)
	thirdCmd := fx.command(t, context.Background(), args...)
	third := run("repeat-default", thirdCmd)
	assertExit(t, third, 0)
	thirdReceipt, err := os.ReadFile(filepath.Join(os.Getenv("SKILLGLASS_EVIDENCE"), third.launch.Path))
	if err != nil {
		t.Fatal(err)
	}
	writeEvidence(t, "investigator/repeated-default-original-receipt.json", thirdReceipt)
	if first.launch.Path == second.launch.Path || second.launch.Path == third.launch.Path {
		t.Error("launch receipt identities collided")
	}
	preserved, err := os.ReadFile(receiptPath)
	if err != nil || !bytes.Equal(preserved, firstReceipt) {
		t.Error("configured launch receipt was replaced")
	}
	var defaultReceipt, emptyReceipt launchReceipt
	decodeJSON(t, secondReceipt, &defaultReceipt)
	for _, entry := range defaultReceipt.Environment {
		if strings.HasPrefix(entry, "CLAUDE_CONFIG_DIR=") {
			t.Error("absent configuration override was invented")
		}
	}
	emptyCmd := fx.command(t, context.Background(), args...)
	emptyCmd.Env = append(fx.environment(), "CLAUDE_CONFIG_DIR=")
	empty := run("empty-override", emptyCmd)
	assertExit(t, empty, 0)
	decodeJSON(t, empty.stdout, &list)
	if claudeAt(t, list, filepath.Dir(defaultManifest)).Availability.State != "available" {
		t.Error("empty override public control changed")
	}
	emptyBytes, err := os.ReadFile(filepath.Join(os.Getenv("SKILLGLASS_EVIDENCE"), empty.launch.Path))
	if err != nil {
		t.Fatal(err)
	}
	decodeJSON(t, emptyBytes, &emptyReceipt)
	if !reflect.DeepEqual(emptyReceipt.Environment, emptyCmd.Env) {
		t.Error("explicitly empty override was lost")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	writeEvidence(t, "investigator/fixture-before.json", before)
	writeEvidence(t, "investigator/fixture-after.json", treeManifest(t, fx.base))
	outcome, _ := json.MarshalIndent(map[string]any{"actual_configured_environment_recorded": environmentCorrect, "actual_configured_cwd_recorded": cwdCorrect, "identical_argv": reflect.DeepEqual(configuredCmd.Args, secondCmd.Args), "configured_receipt_path": receiptPath, "configured_receipt_bytes_equal_default": bytes.Equal(firstReceipt, secondReceipt), "repeat_reuses_same_path_and_bytes": bytes.Equal(secondReceipt, thirdReceipt), "configured_public_literal": "configured-home-literal", "default_public_literal": "default-home-literal", "fixture_unchanged": true}, "", "  ")
	writeEvidence(t, "investigator/results.json", append(outcome, '\n'))
	if !environmentCorrect {
		t.Errorf("receipt environment omits finalized CLAUDE_CONFIG_DIR; want exact command.Env")
	}
	if !cwdCorrect {
		t.Errorf("receipt cwd = %q; want finalized command.Dir %q", receipt.Cwd, configuredCmd.Dir)
	}
	if bytes.Equal(firstReceipt, secondReceipt) {
		t.Errorf("configured and default public invocations overwrite the same receipt path, with identical misrecorded context")
	}
	if bytes.Equal(secondReceipt, thirdReceipt) {
		t.Errorf("repeated invocation retains no immutable distinct launch receipt")
	}
}
