package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

const askPassPromptTestSecret = "the-master-secret"

func TestAskPassCmdDeclinesNonPasswordPromptWithoutConsumingToken(t *testing.T) {
	socketPath := startTestPassphraseAgent(t)

	// maxUses=2: resolveAskPassTokenSecret is documented to *consume* a use, so
	// the verification resolve below burns one use; the real password-prompt
	// invocation needs the second. The decline path itself must consume zero —
	// if it consumed one, only one use would remain and either the verification
	// resolve or the final invocation would fail, catching the regression.
	token, cleanupToken, err := registerAskPassToken(socketPath, askPassPromptTestSecret, time.Minute, 2, "")
	if err != nil {
		t.Fatalf("registerAskPassToken: %v", err)
	}
	defer cleanupToken()

	hostKeyPrompt := "Are you sure you want to continue connecting (yes/no/[fingerprint])? "
	declineCmd := newAskPassCmd(&rootOptions{agentSocket: socketPath})
	var declineOut, declineErr bytes.Buffer
	declineCmd.SetOut(&declineOut)
	declineCmd.SetErr(&declineErr)
	declineCmd.SetArgs([]string{"--token", token, "--", hostKeyPrompt})
	err = declineCmd.Execute()
	if err == nil {
		t.Fatalf("expected askpass command to decline non-password prompt, got nil error (stdout=%q stderr=%q)", declineOut.String(), declineErr.String())
	}
	if !strings.Contains(err.Error(), "declined") {
		t.Fatalf("expected decline error to mention %q, got %q", "declined", err.Error())
	}

	// The single-use token must NOT have been consumed by the declined prompt.
	if _, err := resolveAskPassTokenSecret(socketPath, token, ""); err != nil {
		t.Fatalf("expected token to survive a declined prompt, but resolve failed: %v", err)
	}

	passwordPrompt := "deploy@host's password: "
	answerCmd := newAskPassCmd(&rootOptions{agentSocket: socketPath})
	var answerOut, answerErr bytes.Buffer
	answerCmd.SetOut(&answerOut)
	answerCmd.SetErr(&answerErr)
	answerCmd.SetArgs([]string{"--token", token, "--", passwordPrompt})
	if err := answerCmd.Execute(); err != nil {
		t.Fatalf("expected askpass command to answer password prompt, got error: %v (stderr=%q)", err, answerErr.String())
	}
	if got := strings.TrimSpace(answerOut.String()); got != askPassPromptTestSecret {
		t.Fatalf("expected stdout secret %q, got %q", askPassPromptTestSecret, got)
	}
}

func TestAskPassCmdAnswersPasswordPrompt(t *testing.T) {
	socketPath := startTestPassphraseAgent(t)

	token, cleanupToken, err := registerAskPassToken(socketPath, askPassPromptTestSecret, time.Minute, 1, "")
	if err != nil {
		t.Fatalf("registerAskPassToken: %v", err)
	}
	defer cleanupToken()

	passwordPrompt := "deploy@host's password: "
	cmd := newAskPassCmd(&rootOptions{agentSocket: socketPath})
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"--token", token, "--", passwordPrompt})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected askpass command to succeed, got error: %v (stderr=%q)", err, errBuf.String())
	}
	if got := strings.TrimSpace(out.String()); got != askPassPromptTestSecret {
		t.Fatalf("expected stdout secret %q, got %q", askPassPromptTestSecret, got)
	}
}
