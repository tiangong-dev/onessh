package cli

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newAgentCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage in-memory master-password cache agent",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		newAgentServeCmd(opts),
		newAgentStartCmd(opts),
		newAgentStopCmd(opts),
		newAgentClearAllCmd(opts),
		newAgentStatusCmd(opts),
	)
	return cmd
}

func newAgentServeCmd(opts *rootOptions) *cobra.Command {
	var socket string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run agent server in foreground",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			socketPath, err := resolveAgentSocketPath(resolveSocketFlag(socket, opts))
			if err != nil {
				return err
			}
			return servePassphraseAgentWithCapability(socketPath, cmd.ErrOrStderr(), resolveCapabilityFlag("", opts))
		},
	}
	cmd.Flags().StringVar(&socket, "socket", "", "Agent Unix socket path")
	return cmd
}

func newAgentStartCmd(opts *rootOptions) *cobra.Command {
	var (
		socket   string
		printEnv bool
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start agent server in background",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			socketPath, err := resolveAgentSocketPath(resolveSocketFlag(socket, opts))
			if err != nil {
				return err
			}
			capability := resolveCapabilityFlag("", opts)
			generated := false
			if capability == "" {
				capability, err = generateAgentCapabilityToken()
				if err != nil {
					return err
				}
				generated = true
			}
			if err := startPassphraseAgentProcess(socketPath, capability); err != nil {
				return err
			}
			exportLine := fmt.Sprintf("export %s=%s", onesshAgentCapabilityEnv, shellSingleQuote(capability))
			if printEnv {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), exportLine)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✔ agent started: %s\n", socketPath)
			if generated {
				fmt.Fprintln(cmd.OutOrStdout(), "Generated session capability token (not persisted).")
				fmt.Fprintf(cmd.OutOrStdout(), "Run in current shell: %s\n", exportLine)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&socket, "socket", "", "Agent Unix socket path")
	cmd.Flags().BoolVar(&printEnv, "print-env", false, "Print export command for ONESSH_AGENT_CAPABILITY (for eval)")
	return cmd
}

func newAgentStopCmd(opts *rootOptions) *cobra.Command {
	var socket string

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop agent server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			socketPath, err := resolveAgentSocketPath(resolveSocketFlag(socket, opts))
			if err != nil {
				return err
			}
			if err := requestPassphraseAgentStop(socketPath, resolveCapabilityFlag("", opts)); err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "Agent is not running.")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✔ agent stopped")
			return nil
		},
	}
	cmd.Flags().StringVar(&socket, "socket", "", "Agent Unix socket path")
	return cmd
}

func newAgentStatusCmd(opts *rootOptions) *cobra.Command {
	var socket string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show agent status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			socketPath, err := resolveAgentSocketPath(resolveSocketFlag(socket, opts))
			if err != nil {
				return err
			}
			if err := pingPassphraseAgent(socketPath, resolveCapabilityFlag("", opts)); err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "not running (%s)\n", socketPath)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "running (%s)\n", socketPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&socket, "socket", "", "Agent Unix socket path")
	return cmd
}

func newAgentClearAllCmd(opts *rootOptions) *cobra.Command {
	var socket string

	cmd := &cobra.Command{
		Use:   "clear-all",
		Short: "Clear all cached secrets and askpass tokens",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			socketPath, err := resolveAgentSocketPath(resolveSocketFlag(socket, opts))
			if err != nil {
				return err
			}
			if err := clearPassphraseAgentAll(socketPath, resolveCapabilityFlag("", opts)); err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "Agent is not running.")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✔ agent cache cleared")
			return nil
		},
	}
	cmd.Flags().StringVar(&socket, "socket", "", "Agent Unix socket path")
	return cmd
}

func newAskPassCmd(opts *rootOptions) *cobra.Command {
	var (
		socket string
		token  string
	)

	cmd := &cobra.Command{
		Use:    "askpass",
		Short:  "Internal askpass helper (do not call directly)",
		Args:   cobra.NoArgs,
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			socketValue := strings.TrimSpace(socket)
			if socketValue == "" {
				socketValue = strings.TrimSpace(os.Getenv("ONESSH_ASKPASS_SOCKET"))
			}
			if socketValue == "" {
				socketValue = resolveSocketFlag("", opts)
			}
			socketPath, err := resolveAgentSocketPath(socketValue)
			if err != nil {
				return err
			}

			tokenValue := strings.TrimSpace(token)
			if tokenValue == "" {
				tokenValue = strings.TrimSpace(os.Getenv("ONESSH_ASKPASS_TOKEN"))
			}
			if tokenValue == "" {
				return errors.New("missing askpass token")
			}
			capabilityValue := strings.TrimSpace(os.Getenv("ONESSH_ASKPASS_CAPABILITY"))
			if capabilityValue == "" {
				capabilityValue = resolveCapabilityFlag("", opts)
			}

			secret, err := resolveAskPassTokenSecret(socketPath, tokenValue, capabilityValue)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), secret)
			return err
		},
	}
	cmd.Flags().StringVar(&socket, "socket", "", "Agent Unix socket path")
	cmd.Flags().StringVar(&token, "token", "", "Askpass token")
	_ = cmd.Flags().MarkHidden("socket")
	_ = cmd.Flags().MarkHidden("token")
	return cmd
}

func resolveSocketFlag(explicit string, opts *rootOptions) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if opts == nil {
		return ""
	}
	return opts.agentSocket
}

func resolveCapabilityFlag(explicit string, opts *rootOptions) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	if opts == nil {
		return resolveAgentCapability("")
	}
	return resolveAgentCapability(opts.agentCapability)
}

func generateAgentCapabilityToken() (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generate agent capability token: %w", err)
	}
	return hex.EncodeToString(secret), nil
}
