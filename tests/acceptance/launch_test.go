package acceptance_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/alexdancer/skillglass/tests/launchrecord"
	"github.com/creack/pty"
)

type fixtureCommand struct {
	*exec.Cmd
	fixture        fixture
	record         *launchrecord.Record
	payloadProfile []byte
}

func invocationDirectory() string {
	root := os.Getenv("SKILLGLASS_EVIDENCE")
	if root == "" {
		root = builtBinaryDir
	}
	return filepath.Join(root, "invocations")
}
func (fx fixture) wrap(command *exec.Cmd) *fixtureCommand {
	return &fixtureCommand{Cmd: command, fixture: fx}
}
func (command *fixtureCommand) begin() error {
	if command.record != nil {
		return errors.New("fixture command already launched")
	}
	profile := append([]byte(nil), command.payloadProfile...)
	executable := command.Path
	if filepath.Base(command.Path) == "sandbox-exec" && len(command.Args) >= 4 {
		executable = command.Args[3]
		if command.Args[1] == "-p" {
			profile = []byte(command.Args[2])
		} else if command.Args[1] == "-f" {
			var err error
			profile, err = os.ReadFile(command.Args[2])
			if err != nil {
				return err
			}
		}
	}
	var artifacts []string
	if filepath.Base(executable) == "tmux" {
		for i, arg := range command.Args {
			if arg != "new-session" && arg != "new-window" && arg != "respawn-pane" {
				continue
			}
			if i == len(command.Args)-1 {
				return errors.New("tmux payload missing")
			}
			last := len(command.Args) - 1
			payload := command.Args[last]
			profileMode := ""
			if len(command.payloadProfile) != 0 {
				profileMode = " --deferred-profile"
			}
			command.Args[last] = "exec " + shellQuote(fixtureLaunchBinary) + " --directory " + shellQuote(filepath.Join(command.fixture.tmuxTemporary, "invocations")) + " --profile " + shellQuote(base64.StdEncoding.EncodeToString(profile)) + profileMode + " --artifact " + shellQuote(skillglassBinary) + " -- /bin/sh -c " + shellQuote(payload)
			artifacts = []string{fixtureLaunchBinary, skillglassBinary}
			break
		}
	}
	var err error
	command.record, err = launchrecord.Begin(command.Cmd, invocationDirectory(), nil, artifacts...)
	return err
}
func (command *fixtureCommand) Run() error {
	if err := command.begin(); err != nil {
		return err
	}
	var stdout, stderr bytes.Buffer
	out, errout := command.Stdout, command.Stderr
	if out == nil {
		out = io.Discard
	}
	if errout == nil {
		errout = io.Discard
	}
	command.Stdout = io.MultiWriter(out, &stdout)
	command.Stderr = io.MultiWriter(errout, &stderr)
	err := command.Cmd.Run()
	if receiptErr := command.record.Complete(err, stdout.Bytes(), stderr.Bytes()); receiptErr != nil {
		return errors.Join(err, receiptErr)
	}
	return err
}
func (command *fixtureCommand) Output() ([]byte, error) {
	var stdout bytes.Buffer
	command.Stdout = &stdout
	err := command.Run()
	return stdout.Bytes(), err
}
func (command *fixtureCommand) CombinedOutput() ([]byte, error) {
	if err := command.begin(); err != nil {
		return nil, err
	}
	output, err := command.Cmd.CombinedOutput()
	if receiptErr := command.record.Complete(err, output, nil); receiptErr != nil {
		return output, errors.Join(err, receiptErr)
	}
	return output, err
}
func (command *fixtureCommand) startPTY(size *pty.Winsize) (*os.File, error) {
	if err := command.begin(); err != nil {
		return nil, err
	}
	terminal, err := pty.StartWithSize(command.Cmd, size)
	if err != nil {
		return nil, errors.Join(err, command.record.Complete(err, nil, nil))
	}
	return terminal, nil
}
func (command *fixtureCommand) completePTY(err error, output []byte) error {
	return command.record.Complete(err, output, nil)
}
