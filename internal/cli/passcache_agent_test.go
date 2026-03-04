package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func startTestPassphraseAgent(t *testing.T) string {
	t.Helper()

	socketPath := fmt.Sprintf("/tmp/onessh-agent-%d.sock", time.Now().UnixNano())
	t.Cleanup(func() {
		_ = os.Remove(socketPath)
	})
	errCh := make(chan error, 1)
	go func() {
		errCh <- servePassphraseAgent(socketPath, io.Discard)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		select {
		case err := <-errCh:
			if err == nil {
				t.Fatalf("agent server exited unexpectedly")
			}
			t.Fatalf("agent server failed to start: %v", err)
		default:
		}

		if err := pingPassphraseAgent(socketPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("agent did not start in time at %s", socketPath)
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Cleanup(func() {
		_ = requestPassphraseAgentStop(socketPath)
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("agent server exited with error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("agent server did not exit")
		}
	})
	return socketPath
}

func TestPassphraseAgentClientSetGetClear(t *testing.T) {
	t.Parallel()

	socketPath := startTestPassphraseAgent(t)
	client, err := newPassphraseAgentClient("/tmp/config-a.enc", time.Minute, false, socketPath)
	if err != nil {
		t.Fatalf("newPassphraseAgentClient: %v", err)
	}

	if err := client.Set([]byte("secret-pass")); err != nil {
		t.Fatalf("client.Set: %v", err)
	}

	got, ok, err := client.Get()
	if err != nil {
		t.Fatalf("client.Get: %v", err)
	}
	if !ok {
		t.Fatalf("expected cached value")
	}
	if string(got) != "secret-pass" {
		t.Fatalf("unexpected cached value: %s", string(got))
	}
	wipe(got)

	if err := client.Clear(); err != nil {
		t.Fatalf("client.Clear: %v", err)
	}
	got, ok, err = client.Get()
	if err != nil {
		t.Fatalf("client.Get after clear: %v", err)
	}
	if ok || len(got) != 0 {
		t.Fatalf("expected empty cache after clear")
	}
}

func TestPassphraseAgentClientConfigIsolation(t *testing.T) {
	t.Parallel()

	socketPath := startTestPassphraseAgent(t)
	clientA, err := newPassphraseAgentClient("/tmp/config-a.enc", time.Minute, false, socketPath)
	if err != nil {
		t.Fatalf("newPassphraseAgentClient A: %v", err)
	}
	clientB, err := newPassphraseAgentClient("/tmp/config-b.enc", time.Minute, false, socketPath)
	if err != nil {
		t.Fatalf("newPassphraseAgentClient B: %v", err)
	}

	if err := clientA.Set([]byte("secret-pass")); err != nil {
		t.Fatalf("clientA.Set: %v", err)
	}
	got, ok, err := clientB.Get()
	if err != nil {
		t.Fatalf("clientB.Get: %v", err)
	}
	if ok || len(got) != 0 {
		t.Fatalf("expected empty cache for different config path")
	}
}

func TestPassphraseAgentClientExpiration(t *testing.T) {
	t.Parallel()

	socketPath := startTestPassphraseAgent(t)
	client, err := newPassphraseAgentClient("/tmp/config-a.enc", time.Second, false, socketPath)
	if err != nil {
		t.Fatalf("newPassphraseAgentClient: %v", err)
	}

	if err := client.Set([]byte("secret-pass")); err != nil {
		t.Fatalf("client.Set: %v", err)
	}
	time.Sleep(2100 * time.Millisecond)

	got, ok, err := client.Get()
	if err != nil {
		t.Fatalf("client.Get: %v", err)
	}
	if ok || len(got) != 0 {
		t.Fatalf("expected cache to expire")
	}
}

func TestAskPassTokenLifecycle(t *testing.T) {
	t.Parallel()

	socketPath := startTestPassphraseAgent(t)
	token, cleanup, err := registerAskPassToken(socketPath, "ssh-secret", 2*time.Second, 2)
	if err != nil {
		t.Fatalf("registerAskPassToken: %v", err)
	}
	defer cleanup()

	first, err := resolveAskPassTokenSecret(socketPath, token)
	if err != nil {
		t.Fatalf("resolveAskPassTokenSecret first: %v", err)
	}
	if first != "ssh-secret" {
		t.Fatalf("unexpected first secret: %q", first)
	}

	second, err := resolveAskPassTokenSecret(socketPath, token)
	if err != nil {
		t.Fatalf("resolveAskPassTokenSecret second: %v", err)
	}
	if second != "ssh-secret" {
		t.Fatalf("unexpected second secret: %q", second)
	}

	_, err = resolveAskPassTokenSecret(socketPath, token)
	if err == nil || !strings.Contains(err.Error(), "not found or expired") {
		t.Fatalf("expected token exhaustion error, got %v", err)
	}
}
