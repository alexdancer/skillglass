package acceptance_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

const fixtureScannerPayload = "GET / HTTP/1.1\r\nuser-agent: inert-fc001-scanner\r\n\r\n"

func allowedProbeNetworkCanary(t *testing.T, fx fixture, listener net.Listener) string {
	t.Helper()
	canaryBytes := make([]byte, 32)
	if _, err := rand.Read(canaryBytes); err != nil {
		t.Fatal(err)
	}
	networkCanary := "FC001_" + hex.EncodeToString(canaryBytes)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	positiveCommand := exec.CommandContext(ctx, isolationProbeBinary, "--network-positive", listener.Addr().String(), networkCanary)
	positiveCommand.Dir, positiveCommand.Env = fx.project, fx.environment()
	positiveOutput, err := positiveCommand.CombinedOutput()
	if err != nil {
		t.Fatalf("allowed child network control: %v: %s", err, positiveOutput)
	}
	var positiveChild struct {
		Passed bool
		Sent   int
	}
	if err := json.Unmarshal(positiveOutput, &positiveChild); err != nil || !positiveChild.Passed || positiveChild.Sent != len(networkCanary) {
		t.Fatalf("allowed child payload: %s (%v)", positiveOutput, err)
	}
	positiveObservation, err := collectFixtureNetwork(listener, []byte(networkCanary))
	if err != nil || !positiveObservation.CanarySeen {
		t.Fatalf("receiver missed allowed child canary: %+v (%v)", positiveObservation, err)
	}
	writeEvidence(t, "network-observer/"+t.Name()+"-positive.json", positiveOutput)
	return networkCanary
}

func assertFixtureNetworkCanaryAbsent(t *testing.T, listener net.Listener, networkCanary string) {
	t.Helper()
	// Deterministically retain the unrelated service-discovery regression.
	scanner, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = scanner.SetWriteDeadline(time.Now().Add(time.Second))
	if _, err := io.WriteString(scanner, fixtureScannerPayload); err != nil {
		scanner.Close()
		t.Fatal(err)
	}
	_ = scanner.Close()
	observation, err := collectFixtureNetwork(listener, []byte(networkCanary))
	if err != nil {
		t.Fatal(err)
	}
	if observation.CanarySeen {
		t.Fatal("sandboxed network probe delivered its canary")
	}
	if !observation.ScannerSeen {
		t.Fatal("receiver missed the exact unrelated scanner control")
	}
	observationJSON, err := json.MarshalIndent(observation, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeEvidence(t, "network-observer/"+t.Name()+"-observation.json", observationJSON)
	t.Logf("network observer: %d unrelated connections, %d bytes; no child canary", observation.Connections, observation.Bytes)
}

// Capture is capped in time, connection count, and total bytes. Exhaustion fails closed.
type fixtureNetworkObservation struct {
	Connections int  `json:"connections"`
	Bytes       int  `json:"bytes"`
	CanarySeen  bool `json:"canary_seen"`
	ScannerSeen bool `json:"scanner_seen"`
	PeerResets  int  `json:"peer_resets"`
}

func collectFixtureNetwork(listener net.Listener, canary []byte) (fixtureNetworkObservation, error) {
	var observation fixtureNetworkObservation
	tcp, ok := listener.(*net.TCPListener)
	if !ok {
		return observation, fmt.Errorf("network observer requires TCP listener")
	}
	deadline := time.Now().Add(time.Second)
	if err := tcp.SetDeadline(deadline); err != nil {
		return observation, err
	}
	defer tcp.SetDeadline(time.Time{})
	for observation.Connections < 64 {
		connection, err := tcp.Accept()
		if err != nil {
			if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNABORTED) {
				observation.Connections++
				observation.PeerResets++
				continue
			}
			var networkErr net.Error
			if errors.As(err, &networkErr) && networkErr.Timeout() {
				return observation, nil
			}
			return observation, err
		}
		observation.Connections++
		readDeadline := minTime(deadline, time.Now().Add(100*time.Millisecond))
		if err := connection.SetReadDeadline(readDeadline); err != nil {
			connection.Close()
			return observation, err
		}
		payload, readErr := io.ReadAll(io.LimitReader(connection, int64(65537-observation.Bytes)))
		_ = connection.Close()
		observation.Bytes += len(payload)
		observation.CanarySeen = observation.CanarySeen || bytes.Contains(payload, canary)
		observation.ScannerSeen = observation.ScannerSeen || bytes.Contains(payload, []byte(fixtureScannerPayload))
		if observation.Bytes > 65536 {
			return observation, fmt.Errorf("network observer exceeded 65536-byte cap")
		}
		if errors.Is(readErr, syscall.ECONNRESET) {
			observation.PeerResets++
			continue
		}
		if readErr != nil {
			var networkErr net.Error
			if !errors.As(readErr, &networkErr) || !networkErr.Timeout() {
				return observation, readErr
			}
		}
	}
	return observation, fmt.Errorf("network observer exceeded 64-connection cap")
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

func TestFixtureNetworkObserverAttributesCanary(t *testing.T) {
	for _, test := range []struct {
		name        string
		payload     string
		wantCanary  bool
		wantError   bool
		resetPeer   bool
		wantScanner bool
	}{
		{name: "unrelated-http", payload: fixtureScannerPayload, wantScanner: true},
		{name: "unrelated-reset", resetPeer: true},
		{name: "owned-canary", payload: "fixture-observer-canary", wantCanary: true},
		{name: "byte-cap-fails-closed", payload: strings.Repeat("x", 65537), wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			if test.resetPeer {
				if err := connection.(*net.TCPConn).SetLinger(0); err != nil {
					t.Fatal(err)
				}
			}
			_ = connection.SetWriteDeadline(time.Now().Add(time.Second))
			_, writeErr := io.WriteString(connection, test.payload)
			_ = connection.Close()
			if writeErr != nil {
				t.Fatal(writeErr)
			}
			observation, err := collectFixtureNetwork(listener, []byte("fixture-observer-canary"))
			if (err != nil) != test.wantError {
				t.Fatalf("observation=%+v error=%v", observation, err)
			}
			if test.resetPeer && observation.PeerResets == 0 {
				t.Fatalf("controlled reset not observed: %+v", observation)
			}
			if observation.ScannerSeen != test.wantScanner {
				t.Fatalf("scanner attribution: %+v", observation)
			}
			if observation.CanarySeen != test.wantCanary {
				t.Fatalf("observation=%+v", observation)
			}
		})
	}
}
