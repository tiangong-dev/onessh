package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"sort"
	"strconv"
	"strings"

	"onessh/internal/store"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

type rootOptions struct {
	configPath string
}

func NewRootCmd() *cobra.Command {
	opts := &rootOptions{}

	rootCmd := &cobra.Command{
		Use:           "onessh [host]",
		Short:         "Manage and connect SSH hosts from encrypted config",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return runConnect(cmd, opts, args[0])
		},
	}

	rootCmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "Path to encrypted config file")

	rootCmd.AddCommand(
		newInitCmd(opts),
		newAddCmd(opts),
		newUpdateCmd(opts),
		newRmCmd(opts),
		newListCmd(opts),
		newDumpCmd(opts),
		newConnectCmd(opts),
	)

	return rootCmd
}

func newInitCmd(opts *rootOptions) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize encrypted OneSSH config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := opts.repository()
			if err != nil {
				return err
			}

			if repo.Exists() && !force {
				return fmt.Errorf("config already exists at %s (use --force to overwrite)", repo.Path)
			}

			pass1, err := promptRequiredPassword("Enter master password: ")
			if err != nil {
				return err
			}
			defer wipe(pass1)

			pass2, err := promptRequiredPassword("Confirm master password: ")
			if err != nil {
				return err
			}
			defer wipe(pass2)

			if !bytes.Equal(pass1, pass2) {
				return errors.New("passwords do not match")
			}

			cfg := store.NewPlainConfig()
			if err := repo.Save(cfg, pass1); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✔ onessh configuration initialized: %s\n", repo.Path)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing encrypted config")
	return cmd
}

func newAddCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <host-alias>",
		Short: "Add a host entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias := strings.TrimSpace(args[0])
			if alias == "" {
				return errors.New("host alias cannot be empty")
			}

			repo, err := opts.repository()
			if err != nil {
				return err
			}

			cfg, pass, err := loadConfig(repo)
			if err != nil {
				return err
			}
			defer wipe(pass)

			if _, exists := cfg.Hosts[alias]; exists {
				return fmt.Errorf("host %q already exists (use update)", alias)
			}

			newHost, err := promptHostConfig(nil)
			if err != nil {
				return err
			}
			cfg.Hosts[alias] = newHost

			if err := repo.Save(cfg, pass); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✔ host %s added\n", alias)
			return nil
		},
	}
	return cmd
}

func newUpdateCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <host-alias>",
		Short: "Update an existing host entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias := strings.TrimSpace(args[0])
			if alias == "" {
				return errors.New("host alias cannot be empty")
			}

			repo, err := opts.repository()
			if err != nil {
				return err
			}

			cfg, pass, err := loadConfig(repo)
			if err != nil {
				return err
			}
			defer wipe(pass)

			existing, exists := cfg.Hosts[alias]
			if !exists {
				return fmt.Errorf("host %q does not exist", alias)
			}

			updatedHost, err := promptHostConfig(&existing)
			if err != nil {
				return err
			}
			cfg.Hosts[alias] = updatedHost

			if err := repo.Save(cfg, pass); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✔ host %s updated\n", alias)
			return nil
		},
	}
	return cmd
}

func newRmCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm <host-alias>",
		Short: "Remove a host entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias := strings.TrimSpace(args[0])
			if alias == "" {
				return errors.New("host alias cannot be empty")
			}

			repo, err := opts.repository()
			if err != nil {
				return err
			}

			cfg, pass, err := loadConfig(repo)
			if err != nil {
				return err
			}
			defer wipe(pass)

			if _, exists := cfg.Hosts[alias]; !exists {
				return fmt.Errorf("host %q does not exist", alias)
			}

			delete(cfg.Hosts, alias)

			if err := repo.Save(cfg, pass); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✔ host %s removed\n", alias)
			return nil
		},
	}
	return cmd
}

func newListCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all host aliases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := opts.repository()
			if err != nil {
				return err
			}

			cfg, pass, err := loadConfig(repo)
			if err != nil {
				return err
			}
			defer wipe(pass)

			aliases := make([]string, 0, len(cfg.Hosts))
			for alias := range cfg.Hosts {
				aliases = append(aliases, alias)
			}
			sort.Strings(aliases)

			for _, alias := range aliases {
				fmt.Fprintln(cmd.OutOrStdout(), alias)
			}

			return nil
		},
	}
	return cmd
}

func newDumpCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dump",
		Short: "Dump decrypted YAML to stdout",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := opts.repository()
			if err != nil {
				return err
			}

			cfg, pass, err := loadConfig(repo)
			if err != nil {
				return err
			}
			defer wipe(pass)

			out, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("marshal yaml: %w", err)
			}

			_, err = cmd.OutOrStdout().Write(out)
			return err
		},
	}
	return cmd
}

func newConnectCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect <host-alias>",
		Short: "Connect to a host alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnect(cmd, opts, args[0])
		},
	}
	return cmd
}

func runConnect(cmd *cobra.Command, opts *rootOptions, alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return errors.New("host alias cannot be empty")
	}

	repo, err := opts.repository()
	if err != nil {
		return err
	}

	cfg, pass, err := loadConfig(repo)
	if err != nil {
		return err
	}
	defer wipe(pass)

	target, exists := cfg.Hosts[alias]
	if !exists {
		return fmt.Errorf("host %q not found", alias)
	}

	displayPort := target.Port
	if displayPort <= 0 {
		displayPort = 22
	}
	displayTarget := target.Host
	if target.User != "" {
		displayTarget = fmt.Sprintf("%s@%s", target.User, target.Host)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Connecting to %s:%d...\n", displayTarget, displayPort)
	return executeSSH(target, cmd.ErrOrStderr())
}

func executeSSH(host store.HostConfig, errOut io.Writer) error {
	args := make([]string, 0, 10)

	if host.Port <= 0 {
		host.Port = 22
	}

	args = append(args, "-p", strconv.Itoa(host.Port))

	if host.ProxyJump != "" {
		args = append(args, "-J", host.ProxyJump)
	}

	switch strings.ToLower(host.Auth.Type) {
	case "key":
		if host.Auth.KeyPath != "" {
			keyPath, err := expandTilde(host.Auth.KeyPath)
			if err != nil {
				return err
			}
			args = append(args, "-i", keyPath)
		}
	case "password":
	default:
		return fmt.Errorf("unsupported auth type: %s", host.Auth.Type)
	}

	destination := host.Host
	if host.User != "" {
		destination = fmt.Sprintf("%s@%s", host.User, host.Host)
	}
	args = append(args, destination)

	binary := "ssh"
	env := os.Environ()
	if strings.ToLower(host.Auth.Type) == "password" && host.Auth.Password != "" {
		if _, err := exec.LookPath("sshpass"); err == nil {
			binary = "sshpass"
			args = append([]string{"-e", "ssh"}, args...)
			env = append(env, "SSHPASS="+host.Auth.Password)
		} else {
			fmt.Fprintln(errOut, "sshpass not found, ssh will prompt password interactively.")
		}
	}

	execCmd := exec.Command(binary, args...)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Env = env
	return execCmd.Run()
}

func loadConfig(repo store.Repository) (store.PlainConfig, []byte, error) {
	passphrase, err := promptRequiredPassword("Enter master password: ")
	if err != nil {
		return store.PlainConfig{}, nil, err
	}

	cfg, err := repo.Load(passphrase)
	if err != nil {
		wipe(passphrase)
		if errors.Is(err, store.ErrConfigNotFound) {
			return store.PlainConfig{}, nil, fmt.Errorf("%w (run `onessh init` first)", err)
		}
		return store.PlainConfig{}, nil, err
	}
	return cfg, passphrase, nil
}

func promptHostConfig(existing *store.HostConfig) (store.HostConfig, error) {
	inputReader := bufio.NewReader(os.Stdin)
	defaultUser := currentUserName()

	defaultHost := ""
	defaultPort := 22
	defaultAuthType := "key"
	defaultKeyPath := "~/.ssh/id_ed25519"
	defaultProxyJump := ""
	defaultPassword := ""
	defaultEnv := map[string]string{}

	if existing != nil {
		defaultHost = existing.Host
		if existing.User != "" {
			defaultUser = existing.User
		}
		if existing.Port > 0 {
			defaultPort = existing.Port
		}
		if existing.Auth.Type != "" {
			defaultAuthType = strings.ToLower(existing.Auth.Type)
		}
		if existing.Auth.KeyPath != "" {
			defaultKeyPath = existing.Auth.KeyPath
		}
		defaultProxyJump = existing.ProxyJump
		defaultPassword = existing.Auth.Password
		defaultEnv = existing.Env
	}

	host, err := promptNonEmpty(inputReader, "Host IP/Domain", defaultHost)
	if err != nil {
		return store.HostConfig{}, err
	}
	userName, err := promptNonEmpty(inputReader, "User", defaultUser)
	if err != nil {
		return store.HostConfig{}, err
	}
	port, err := promptPort(inputReader, defaultPort)
	if err != nil {
		return store.HostConfig{}, err
	}

	authType, err := promptAuthType(inputReader, defaultAuthType)
	if err != nil {
		return store.HostConfig{}, err
	}

	auth := store.AuthConfig{Type: authType}
	switch authType {
	case "key":
		keyPath, err := promptNonEmpty(inputReader, "Key path", defaultKeyPath)
		if err != nil {
			return store.HostConfig{}, err
		}
		auth.KeyPath = keyPath
	case "password":
		if existing != nil && defaultPassword != "" {
			password, changed, err := promptOptionalSecret("Password (press Enter to keep current): ")
			if err != nil {
				return store.HostConfig{}, err
			}
			if changed {
				auth.Password = string(password)
				wipe(password)
			} else {
				auth.Password = defaultPassword
			}
		} else {
			password, err := promptRequiredPassword("Password: ")
			if err != nil {
				return store.HostConfig{}, err
			}
			auth.Password = string(password)
			wipe(password)
		}
	default:
		return store.HostConfig{}, fmt.Errorf("unsupported auth type: %s", authType)
	}

	proxyJump, err := promptOptional(inputReader, "Proxy jump", defaultProxyJump)
	if err != nil {
		return store.HostConfig{}, err
	}

	return store.HostConfig{
		Host:      host,
		User:      userName,
		Port:      port,
		Auth:      auth,
		ProxyJump: proxyJump,
		Env:       defaultEnv,
	}, nil
}

func promptNonEmpty(reader *bufio.Reader, label, defaultValue string) (string, error) {
	for {
		value, err := promptOptional(reader, label, defaultValue)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), nil
		}
	}
}

func promptOptional(reader *bufio.Reader, label, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	text := strings.TrimSpace(line)
	if text == "" {
		return defaultValue, nil
	}
	return text, nil
}

func promptPort(reader *bufio.Reader, defaultPort int) (int, error) {
	for {
		raw, err := promptOptional(reader, "Port", strconv.Itoa(defaultPort))
		if err != nil {
			return 0, err
		}
		port, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || port <= 0 || port > 65535 {
			fmt.Fprintln(os.Stderr, "Port must be a number between 1 and 65535.")
			continue
		}
		return port, nil
	}
}

func promptAuthType(reader *bufio.Reader, defaultType string) (string, error) {
	for {
		raw, err := promptOptional(reader, "Auth type (key/password or 1/2)", defaultType)
		if err != nil {
			return "", err
		}
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "1", "k", "key":
			return "key", nil
		case "2", "p", "pass", "password":
			return "password", nil
		default:
			fmt.Fprintln(os.Stderr, "Auth type must be key/password or 1/2.")
		}
	}
}

func promptRequiredPassword(prompt string) ([]byte, error) {
	fmt.Fprint(os.Stderr, prompt)
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	secret = bytes.TrimSpace(secret)
	if len(secret) == 0 {
		return nil, errors.New("password cannot be empty")
	}
	return secret, nil
}

func promptOptionalSecret(prompt string) ([]byte, bool, error) {
	fmt.Fprint(os.Stderr, prompt)
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, false, err
	}
	secret = bytes.TrimSpace(secret)
	if len(secret) == 0 {
		return nil, false, nil
	}
	return secret, true, nil
}

func (o *rootOptions) repository() (store.Repository, error) {
	path, err := store.ResolvePath(o.configPath)
	if err != nil {
		return store.Repository{}, err
	}
	return store.Repository{Path: path}, nil
}

func currentUserName() string {
	u, err := user.Current()
	if err != nil {
		return "root"
	}
	if u.Username == "" {
		return "root"
	}
	return u.Username
}

func wipe(data []byte) {
	for i := range data {
		data[i] = 0
	}
}

func expandTilde(input string) (string, error) {
	if input == "" {
		return "", nil
	}
	if input == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(input, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return homeDir + "/" + strings.TrimPrefix(input, "~/"), nil
	}
	return input, nil
}
