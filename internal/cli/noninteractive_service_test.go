package cli

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"onessh/internal/domain"
	"onessh/internal/store"
)

func TestHostCLIUpdateAndRemoveNonInteractiveBehavior(t *testing.T) {
	passphrase := []byte("master-pass")
	cfg := domain.NewPlainConfig()
	cfg.Users["ops"] = domain.UserConfig{
		Name: "ubuntu",
		Auth: domain.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"},
	}
	cfg.Hosts["web1"] = domain.HostConfig{
		Host:    "web1.example.com",
		UserRef: "ops",
		Port:    22,
		Env:     map[string]string{"KEEP": "1", "DROP": "1"},
		Tags:    []string{"old", "web"},
	}
	opts, repo := prepareNonInteractiveCLITest(t, cfg, passphrase)

	var updateOut, updateErr bytes.Buffer
	updateCmd := newUpdateCmd(opts)
	updateCmd.SetOut(&updateOut)
	updateCmd.SetErr(&updateErr)
	updateCmd.SetArgs([]string{
		"web1",
		"--alias", "web2",
		"--host", " 10.0.0.2 ",
		"--port", "2200",
		"--proxy-jump", " jump ",
		"--description", " primary web ",
		"--env", "KEEP=2",
		"--env", "NEW=value",
		"--unset-env", "DROP",
		"--pre-connect", " cd /srv/app ",
		"--post-connect", " echo done ",
		"--tag", " Prod ",
		"--untag", "old",
		"--user", " root ",
		"--auth-type", "password",
		"--password", "secret",
	})
	if err := updateCmd.Execute(); err != nil {
		t.Fatalf("host update command: %v", err)
	}
	if got, want := updateOut.String(), "✔ host web1 renamed to web2 and updated\n"; got != want {
		t.Fatalf("host update output = %q, want %q", got, want)
	}
	if !strings.Contains(updateErr.String(), "--password is insecure") {
		t.Fatalf("expected --password insecure warning on stderr, got %q", updateErr.String())
	}

	updated, err := repo.Load(passphrase)
	if err != nil {
		t.Fatalf("load updated config: %v", err)
	}
	if _, exists := updated.Hosts["web1"]; exists {
		t.Fatalf("old host alias still exists: %#v", updated.Hosts)
	}
	host := updated.Hosts["web2"]
	if host.Host != "10.0.0.2" || host.Port != 2200 || host.ProxyJump != "jump" || host.Description != "primary web" {
		t.Fatalf("unexpected updated host: %#v", host)
	}
	if !reflect.DeepEqual(host.Env, map[string]string{"KEEP": "2", "NEW": "value"}) {
		t.Fatalf("host env = %#v", host.Env)
	}
	if !reflect.DeepEqual(host.PreConnect, []string{"cd /srv/app"}) {
		t.Fatalf("host pre_connect = %#v", host.PreConnect)
	}
	if !reflect.DeepEqual(host.PostConnect, []string{"echo done"}) {
		t.Fatalf("host post_connect = %#v", host.PostConnect)
	}
	if !reflect.DeepEqual(host.Tags, []string{"prod", "web"}) {
		t.Fatalf("host tags = %#v", host.Tags)
	}
	if got := updated.Users["ops"]; !reflect.DeepEqual(got, domain.UserConfig{Name: "root", Auth: domain.AuthConfig{Type: "password", Password: "secret"}}) {
		t.Fatalf("linked user = %#v", got)
	}

	var removeOut bytes.Buffer
	removeCmd := newRmCmd(opts)
	removeCmd.SetOut(&removeOut)
	removeCmd.SetErr(&removeOut)
	removeCmd.SetArgs([]string{"web2"})
	if err := removeCmd.Execute(); err != nil {
		t.Fatalf("host remove command: %v", err)
	}
	if got, want := removeOut.String(), "✔ host web2 removed\n"; got != want {
		t.Fatalf("host remove output = %q, want %q", got, want)
	}

	removed, err := repo.Load(passphrase)
	if err != nil {
		t.Fatalf("load removed config: %v", err)
	}
	if _, exists := removed.Hosts["web2"]; exists {
		t.Fatalf("host was not removed: %#v", removed.Hosts)
	}
	if _, exists := removed.Users["ops"]; !exists {
		t.Fatalf("non-interactive host remove deleted linked user profile")
	}
}

func TestHostCLIUpdateNonInteractivePreservesValidationMessages(t *testing.T) {
	passphrase := []byte("master-pass")
	cfg := domain.NewPlainConfig()
	cfg.Users["ops"] = domain.UserConfig{Name: "ubuntu", Auth: domain.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"}}
	cfg.Hosts["web1"] = domain.HostConfig{Host: "web1.example.com", UserRef: "ops", Port: 22}
	opts, _ := prepareNonInteractiveCLITest(t, cfg, passphrase)

	cmd := newUpdateCmd(opts)
	cmd.SetArgs([]string{"web1", "--alias", " "})
	err := cmd.Execute()
	if err == nil || err.Error() != "--alias cannot be empty" {
		t.Fatalf("expected --alias validation message, got %v", err)
	}
}

func TestUserCLIAddUpdateAndRemoveNonInteractiveBehavior(t *testing.T) {
	passphrase := []byte("master-pass")
	opts, repo := prepareNonInteractiveCLITest(t, domain.NewPlainConfig(), passphrase)

	var addOut bytes.Buffer
	addCmd := newUserAddCmd(opts)
	addCmd.SetOut(&addOut)
	addCmd.SetErr(&addOut)
	addCmd.SetArgs([]string{
		" Ops_User ",
		"--name", " ubuntu ",
		"--auth-type", "key",
		"--key-path", " ~/.ssh/id_ed25519 ",
	})
	if err := addCmd.Execute(); err != nil {
		t.Fatalf("user add command: %v", err)
	}
	if got, want := addOut.String(), "✔ user profile ops_user added (ubuntu)\n"; got != want {
		t.Fatalf("user add output = %q, want %q", got, want)
	}

	var updateOut, updateErr bytes.Buffer
	updateCmd := newUserUpdateCmd(opts)
	updateCmd.SetOut(&updateOut)
	updateCmd.SetErr(&updateErr)
	updateCmd.SetArgs([]string{
		"ops_user",
		"--name", " root ",
		"--auth-type", "password",
		"--password", "secret",
	})
	if err := updateCmd.Execute(); err != nil {
		t.Fatalf("user update command: %v", err)
	}
	if got, want := updateOut.String(), "✔ user profile ops_user updated\n"; got != want {
		t.Fatalf("user update output = %q, want %q", got, want)
	}
	if !strings.Contains(updateErr.String(), "--password is insecure") {
		t.Fatalf("expected --password insecure warning on stderr, got %q", updateErr.String())
	}

	updated, err := repo.Load(passphrase)
	if err != nil {
		t.Fatalf("load updated config: %v", err)
	}
	if got := updated.Users["ops_user"]; !reflect.DeepEqual(got, domain.UserConfig{Name: "root", Auth: domain.AuthConfig{Type: "password", Password: "secret"}}) {
		t.Fatalf("updated user = %#v", got)
	}

	var removeOut bytes.Buffer
	removeCmd := newUserRmCmd(opts)
	removeCmd.SetOut(&removeOut)
	removeCmd.SetErr(&removeOut)
	removeCmd.SetArgs([]string{"ops_user"})
	if err := removeCmd.Execute(); err != nil {
		t.Fatalf("user remove command: %v", err)
	}
	if got, want := removeOut.String(), "✔ user profile ops_user removed\n"; got != want {
		t.Fatalf("user remove output = %q, want %q", got, want)
	}
}

func TestUserCLIRemoveNonInteractivePreservesInUseError(t *testing.T) {
	passphrase := []byte("master-pass")
	cfg := domain.NewPlainConfig()
	cfg.Users["ops"] = domain.UserConfig{Name: "ubuntu", Auth: domain.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"}}
	cfg.Hosts["web1"] = domain.HostConfig{Host: "web1.example.com", UserRef: "ops", Port: 22}
	opts, _ := prepareNonInteractiveCLITest(t, cfg, passphrase)

	cmd := newUserRmCmd(opts)
	cmd.SetArgs([]string{"ops"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected user remove to fail")
	}
	want := `user profile "ops" is used by host(s): web1. Please remove these hosts first`
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("user remove error = %q, want to contain %q", err.Error(), want)
	}
}

func prepareNonInteractiveCLITest(t *testing.T, cfg domain.PlainConfig, passphrase []byte) (*rootOptions, store.Repository) {
	t.Helper()

	dataPath := t.TempDir()
	repo := store.Repository{Path: dataPath}
	if err := repo.Save(cfg, passphrase); err != nil {
		t.Fatalf("save initial config: %v", err)
	}

	socketPath := startTestPassphraseAgent(t)
	client, err := newPassphraseAgentClient(passphraseCacheKey(repo.Path), time.Minute, false, socketPath, "")
	if err != nil {
		t.Fatalf("new passphrase cache client: %v", err)
	}
	if err := client.Set(passphrase); err != nil {
		t.Fatalf("cache passphrase: %v", err)
	}

	return &rootOptions{
		dataPath:    dataPath,
		cacheTTL:    time.Minute,
		agentSocket: socketPath,
	}, repo
}
