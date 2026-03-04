package cli

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultAgentDialTimeout = 300 * time.Millisecond
	defaultAgentReqTimeout  = 2 * time.Second
)

const (
	agentActionPing  = "ping"
	agentActionGet   = "get"
	agentActionSet   = "set"
	agentActionClear = "clear"
	agentActionStop  = "stop"
)

type passphraseAgentClient struct {
	socketPath string
	ttl        time.Duration
	configPath string
}

type passphraseAgentRequest struct {
	Action     string `json:"action"`
	ConfigPath string `json:"config_path,omitempty"`
	SecretB64  string `json:"secret_b64,omitempty"`
	ExpiresAt  int64  `json:"expires_at,omitempty"`
}

type passphraseAgentResponse struct {
	OK        bool   `json:"ok"`
	Found     bool   `json:"found,omitempty"`
	SecretB64 string `json:"secret_b64,omitempty"`
	Error     string `json:"error,omitempty"`
}

type passphraseAgentState struct {
	mu      sync.Mutex
	entries map[string]agentEntry
}

type agentEntry struct {
	secret    []byte
	expiresAt int64
}

func newPassphraseAgentClient(
	configPath string,
	ttl time.Duration,
	disabled bool,
	customSocket string,
) (passphraseAgentClient, error) {
	if disabled {
		return passphraseAgentClient{}, nil
	}
	socketPath, err := resolveAgentSocketPath(customSocket)
	if err != nil {
		return passphraseAgentClient{}, err
	}
	return passphraseAgentClient{
		socketPath: socketPath,
		ttl:        normalizeTTL(ttl),
		configPath: configPath,
	}, nil
}

func (c passphraseAgentClient) IsEnabled() bool {
	return strings.TrimSpace(c.socketPath) != ""
}

func (c passphraseAgentClient) Get() ([]byte, bool, error) {
	if !c.IsEnabled() {
		return nil, false, nil
	}
	resp, err := c.request(passphraseAgentRequest{
		Action:     agentActionGet,
		ConfigPath: c.configPath,
	})
	if err != nil {
		return nil, false, nil
	}
	if !resp.Found {
		return nil, false, nil
	}
	secret, err := base64.StdEncoding.DecodeString(resp.SecretB64)
	if err != nil || len(secret) == 0 {
		return nil, false, nil
	}
	return secret, true, nil
}

func (c passphraseAgentClient) Set(passphrase []byte) error {
	if !c.IsEnabled() || len(passphrase) == 0 {
		return nil
	}

	req := passphraseAgentRequest{
		Action:     agentActionSet,
		ConfigPath: c.configPath,
		SecretB64:  base64.StdEncoding.EncodeToString(passphrase),
		ExpiresAt:  time.Now().Add(c.ttl).Unix(),
	}
	if _, err := c.request(req); err == nil {
		return nil
	}

	if err := startPassphraseAgentProcess(c.socketPath); err != nil {
		return err
	}
	_, err := c.request(req)
	return err
}

func (c passphraseAgentClient) Clear() error {
	if !c.IsEnabled() {
		return nil
	}
	_, err := c.request(passphraseAgentRequest{
		Action:     agentActionClear,
		ConfigPath: c.configPath,
	})
	if err != nil {
		return nil
	}
	return nil
}

func (c passphraseAgentClient) request(req passphraseAgentRequest) (passphraseAgentResponse, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, defaultAgentDialTimeout)
	if err != nil {
		return passphraseAgentResponse{}, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(defaultAgentReqTimeout))

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)
	if err := encoder.Encode(req); err != nil {
		return passphraseAgentResponse{}, err
	}

	var resp passphraseAgentResponse
	if err := decoder.Decode(&resp); err != nil {
		return passphraseAgentResponse{}, err
	}
	if !resp.OK {
		if strings.TrimSpace(resp.Error) == "" {
			return passphraseAgentResponse{}, errors.New("agent request failed")
		}
		return passphraseAgentResponse{}, errors.New(resp.Error)
	}
	return resp, nil
}

func resolveAgentSocketPath(custom string) (string, error) {
	if strings.TrimSpace(custom) != "" {
		return expandTilde(custom)
	}
	if fromEnv := strings.TrimSpace(os.Getenv("ONESSH_AGENT_SOCKET")); fromEnv != "" {
		return expandTilde(fromEnv)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(homeDir, ".onessh", "agent.sock"), nil
}

func startPassphraseAgentProcess(socketPath string) error {
	if err := pingPassphraseAgent(socketPath); err == nil {
		return nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	cmd := exec.Command(exePath, "agent", "serve", "--socket", socketPath)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start agent process: %w", err)
	}
	_ = cmd.Process.Release()

	lastErr := error(nil)
	for i := 0; i < 40; i++ {
		time.Sleep(25 * time.Millisecond)
		lastErr = pingPassphraseAgent(socketPath)
		if lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("wait for agent startup: %w", lastErr)
}

func pingPassphraseAgent(socketPath string) error {
	client := passphraseAgentClient{socketPath: socketPath}
	_, err := client.request(passphraseAgentRequest{Action: agentActionPing})
	return err
}

func requestPassphraseAgentStop(socketPath string) error {
	client := passphraseAgentClient{socketPath: socketPath}
	_, err := client.request(passphraseAgentRequest{Action: agentActionStop})
	return err
}

func servePassphraseAgent(socketPath string, errOut io.Writer) error {
	if strings.TrimSpace(socketPath) == "" {
		return errors.New("empty agent socket path")
	}

	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return fmt.Errorf("create agent directory: %w", err)
	}

	if info, err := os.Stat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("agent socket path exists and is not a socket: %s", socketPath)
		}
		if err := pingPassphraseAgent(socketPath); err == nil {
			return fmt.Errorf("agent already running at %s", socketPath)
		}
		if err := os.Remove(socketPath); err != nil {
			return fmt.Errorf("remove stale socket %s: %w", socketPath, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat socket path %s: %w", socketPath, err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}()
	if err := os.Chmod(socketPath, 0o600); err != nil {
		return fmt.Errorf("chmod socket %s: %w", socketPath, err)
	}

	state := &passphraseAgentState{
		entries: map[string]agentEntry{},
	}

	stopCh := make(chan struct{})
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			close(stopCh)
			_ = listener.Close()
		})
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			stop()
		case <-stopCh:
		}
	}()

	for {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			select {
			case <-stopCh:
				return nil
			default:
				if errOut != nil {
					fmt.Fprintf(errOut, "agent accept error: %v\n", acceptErr)
				}
				continue
			}
		}
		go handlePassphraseAgentConn(conn, state, stop)
	}
}

func handlePassphraseAgentConn(conn net.Conn, state *passphraseAgentState, stopFn func()) {
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(defaultAgentReqTimeout))

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	if err := ensureSameUIDClient(conn); err != nil {
		_ = encoder.Encode(passphraseAgentResponse{
			OK:    false,
			Error: err.Error(),
		})
		return
	}

	var req passphraseAgentRequest
	if err := decoder.Decode(&req); err != nil {
		_ = encoder.Encode(passphraseAgentResponse{
			OK:    false,
			Error: "invalid request",
		})
		return
	}

	resp := state.apply(req)
	_ = encoder.Encode(resp)

	if req.Action == agentActionStop && resp.OK {
		stopFn()
	}
}

func ensureSameUIDClient(conn net.Conn) error {
	peerUID, err := socketPeerUID(conn)
	if err != nil {
		return fmt.Errorf("forbidden: cannot verify peer uid (%w)", err)
	}
	currentUID := uint32(os.Getuid())
	if peerUID != currentUID {
		return fmt.Errorf("forbidden: peer uid %d does not match agent uid %d", peerUID, currentUID)
	}
	return nil
}

func (s *passphraseAgentState) apply(req passphraseAgentRequest) passphraseAgentResponse {
	switch req.Action {
	case agentActionPing:
		return passphraseAgentResponse{OK: true}
	case agentActionGet:
		return s.handleGet(req)
	case agentActionSet:
		return s.handleSet(req)
	case agentActionClear:
		return s.handleClear(req)
	case agentActionStop:
		s.clearAll()
		return passphraseAgentResponse{OK: true}
	default:
		return passphraseAgentResponse{
			OK:    false,
			Error: "unsupported action",
		}
	}
}

func (s *passphraseAgentState) handleGet(req passphraseAgentRequest) passphraseAgentResponse {
	configPath := strings.TrimSpace(req.ConfigPath)
	if configPath == "" {
		return passphraseAgentResponse{OK: false, Error: "config_path is required"}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[configPath]
	if !ok {
		return passphraseAgentResponse{OK: true, Found: false}
	}
	if entry.expiresAt <= time.Now().Unix() {
		wipe(entry.secret)
		delete(s.entries, configPath)
		return passphraseAgentResponse{OK: true, Found: false}
	}
	return passphraseAgentResponse{
		OK:        true,
		Found:     true,
		SecretB64: base64.StdEncoding.EncodeToString(entry.secret),
	}
}

func (s *passphraseAgentState) handleSet(req passphraseAgentRequest) passphraseAgentResponse {
	configPath := strings.TrimSpace(req.ConfigPath)
	if configPath == "" {
		return passphraseAgentResponse{OK: false, Error: "config_path is required"}
	}
	if req.ExpiresAt <= time.Now().Unix() {
		return passphraseAgentResponse{OK: false, Error: "expires_at must be in the future"}
	}

	secret, err := base64.StdEncoding.DecodeString(req.SecretB64)
	if err != nil || len(secret) == 0 {
		return passphraseAgentResponse{OK: false, Error: "invalid secret_b64"}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if old, ok := s.entries[configPath]; ok {
		wipe(old.secret)
	}
	s.entries[configPath] = agentEntry{
		secret:    secret,
		expiresAt: req.ExpiresAt,
	}
	return passphraseAgentResponse{OK: true}
}

func (s *passphraseAgentState) handleClear(req passphraseAgentRequest) passphraseAgentResponse {
	configPath := strings.TrimSpace(req.ConfigPath)

	s.mu.Lock()
	defer s.mu.Unlock()

	if configPath == "" {
		for key, entry := range s.entries {
			wipe(entry.secret)
			delete(s.entries, key)
		}
		return passphraseAgentResponse{OK: true}
	}

	if entry, ok := s.entries[configPath]; ok {
		wipe(entry.secret)
		delete(s.entries, configPath)
	}
	return passphraseAgentResponse{OK: true}
}

func (s *passphraseAgentState) clearAll() {
	_ = s.handleClear(passphraseAgentRequest{})
}

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
			return servePassphraseAgent(socketPath, cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&socket, "socket", "", "Agent Unix socket path")
	return cmd
}

func newAgentStartCmd(opts *rootOptions) *cobra.Command {
	var socket string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start agent server in background",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			socketPath, err := resolveAgentSocketPath(resolveSocketFlag(socket, opts))
			if err != nil {
				return err
			}
			if err := startPassphraseAgentProcess(socketPath); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✔ agent started: %s\n", socketPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&socket, "socket", "", "Agent Unix socket path")
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
			if err := requestPassphraseAgentStop(socketPath); err != nil {
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
			if err := pingPassphraseAgent(socketPath); err != nil {
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

func resolveSocketFlag(explicit string, opts *rootOptions) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if opts == nil {
		return ""
	}
	return opts.agentSocket
}
