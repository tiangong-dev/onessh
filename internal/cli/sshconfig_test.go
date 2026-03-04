package cli

import (
	"strings"
	"testing"
)

func TestUpsertSSHConfigManagedBlock(t *testing.T) {
	t.Parallel()

	managed := sshConfigManagedStart + "\nHost web1\n  HostName 1.2.3.4\n" + sshConfigManagedEnd + "\n"

	inserted := upsertSSHConfigManagedBlock("Host old\n  HostName 127.0.0.1\n", managed)
	if !strings.Contains(inserted, sshConfigManagedStart) {
		t.Fatalf("expected managed block to be inserted")
	}

	replaced := upsertSSHConfigManagedBlock(inserted, managed)
	if strings.Count(replaced, sshConfigManagedStart) != 1 {
		t.Fatalf("expected managed block to be replaced in place")
	}
}

func TestParseSSHConfig(t *testing.T) {
	t.Parallel()

	content := `
Host web1
  HostName 10.0.0.10
  User ubuntu
  Port 2222
  ProxyJump bastion
  IdentityFile ~/.ssh/id_ed25519
  SetEnv AWS_PROFILE=prod HTTPS_PROXY=http://127.0.0.1:7890

Host *.internal
  User ignored
`
	entries := parseSSHConfig(content)
	if len(entries) != 1 {
		t.Fatalf("expected 1 parsed entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry.Alias != "web1" {
		t.Fatalf("unexpected alias: %s", entry.Alias)
	}
	if entry.Port != 2222 {
		t.Fatalf("unexpected port: %d", entry.Port)
	}
	if entry.Env["AWS_PROFILE"] != "prod" {
		t.Fatalf("expected AWS_PROFILE env")
	}
}

