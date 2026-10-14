package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/alexdancer/skillglass/tests/launchrecord"
	"github.com/charmbracelet/x/term"
	"github.com/creack/pty"
)

func main() { os.Exit(run()) }
func run() int {
	directory := flag.String("directory", "", "")
	encoded := flag.String("profile", "", "")
	artifact := flag.String("artifact", "", "")
	deferred := flag.Bool("deferred-profile", false, "policy is applied by the payload rather than inherited")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "missing payload")
		return 125
	}
	raw, err := base64.StdEncoding.DecodeString(*encoded)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 125
	}
	profile := &launchrecord.Profile{Bytes: raw, SHA256: launchrecord.Digest(raw), InheritedFrom: launchrecord.Digest(raw)}
	if *deferred {
		profile.InheritedFrom = ""
	}
	if len(raw) == 0 {
		profile = nil
	}
	command := exec.Command(args[0], args[1:]...)
	command.Env = os.Environ()
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 125
	}
	record, err := launchrecord.Begin(command, *directory, profile, *artifact, self)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 125
	}
	output, err := os.OpenFile(filepath.Join(*directory, record.Identity.ID+".stdout"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		record.Complete(err, nil, nil)
		fmt.Fprintln(os.Stderr, err)
		return 125
	}
	defer output.Close()
	size, err := pty.GetsizeFull(os.Stdin)
	if err != nil {
		record.Complete(err, nil, nil)
		fmt.Fprintln(os.Stderr, err)
		return 125
	}
	state, err := term.MakeRaw(os.Stdin.Fd())
	if err != nil {
		record.Complete(err, nil, nil)
		fmt.Fprintln(os.Stderr, err)
		return 125
	}
	defer term.Restore(os.Stdin.Fd(), state)
	terminal, err := pty.StartWithSize(command, size)
	if err != nil {
		record.Complete(err, nil, nil)
		fmt.Fprintln(os.Stderr, err)
		return 125
	}
	defer terminal.Close()
	changes := make(chan os.Signal, 1)
	signal.Notify(changes, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(changes)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			case change := <-changes:
				if change == syscall.SIGWINCH {
					if size, err := pty.GetsizeFull(os.Stdin); err == nil {
						_ = pty.Setsize(terminal, size)
					}
				} else {
					_ = syscall.Kill(-command.Process.Pid, change.(syscall.Signal))
				}
			}
		}
	}()
	go func() { _, _ = io.Copy(terminal, os.Stdin) }()
	digest := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(os.Stdout, digest, output), terminal)
	var terminalError *os.PathError
	terminalEOF := errors.As(copyErr, &terminalError) && terminalError.Op == "read" && terminalError.Path == terminal.Name() && errors.Is(terminalError, syscall.EIO)
	if copyErr != nil && !terminalEOF {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		_ = terminal.Close()
	} else {
		copyErr = nil
	}
	err = command.Wait()
	copyErr = errors.Join(copyErr, output.Sync())
	code := 0
	message := ""
	if err != nil {
		code = command.ProcessState.ExitCode()
		message = err.Error()
	}
	if copyErr != nil {
		code = 125
		message = errors.Join(err, copyErr).Error()
	}
	if receiptErr := record.CompleteHashes(code, message, hex.EncodeToString(digest.Sum(nil)), launchrecord.Digest(nil)); receiptErr != nil {
		fmt.Fprintln(os.Stderr, receiptErr)
		return 125
	}
	return code
}
