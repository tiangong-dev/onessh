package cli

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
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

	agentActionSetAskPass   = "set_askpass"
	agentActionGetAskPass   = "get_askpass"
	agentActionClearAskPass = "clear_askpass"
)

const (
	defaultAskPassTTL     = 2 * time.Minute
	defaultAskPassMaxUses = 8
	askPassTokenBytes     = 24
)

type passphraseAgentClient struct {
	socketPath string
	ttl        time.Duration
	dataPath   string
}

type passphraseAgentRequest struct {
	Action    string `json:"action"`
	DataPath  string `json:"data_path,omitempty"`
	SecretB64 string `json:"secret_b64,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	Token     string `json:"token,omitempty"`
	MaxUses   int    `json:"max_uses,omitempty"`
}

type passphraseAgentResponse struct {
	OK        bool   `json:"ok"`
	Found     bool   `json:"found,omitempty"`
	SecretB64 string `json:"secret_b64,omitempty"`
	Error     string `json:"error,omitempty"`
}

type passphraseAgentState struct {
	mu             sync.Mutex
	entries        map[string]agentEntry
	askpassEntries map[string]askpassEntry
}

type agentEntry struct {
	secret    []byte
	expiresAt int64
}

type askpassEntry struct {
	secret    []byte
	expiresAt int64
	remaining int
}

func newPassphraseAgentClient(
	dataPath string,
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
		dataPath:   dataPath,
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
		Action:   agentActionGet,
		DataPath: c.dataPath,
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
		Action:    agentActionSet,
		DataPath:  c.dataPath,
		SecretB64: base64.StdEncoding.EncodeToString(passphrase),
		ExpiresAt: time.Now().Add(c.ttl).Unix(),
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
		Action:   agentActionClear,
		DataPath: c.dataPath,
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

func registerAskPassToken(socketPath, password string, ttl time.Duration, maxUses int) (string, func(), error) {
	if strings.TrimSpace(password) == "" {
		return "", nil, errors.New("password auth requires non-empty password")
	}
	if ttl <= 0 {
		ttl = defaultAskPassTTL
	}
	if maxUses <= 0 {
		maxUses = defaultAskPassMaxUses
	}

	if err := startPassphraseAgentProcess(socketPath); err != nil {
		return "", nil, err
	}

	token, err := newAskPassToken()
	if err != nil {
		return "", nil, err
	}
	client := passphraseAgentClient{socketPath: socketPath}
	_, err = client.request(passphraseAgentRequest{
		Action:    agentActionSetAskPass,
		Token:     token,
		SecretB64: base64.StdEncoding.EncodeToString([]byte(password)),
		ExpiresAt: time.Now().Add(ttl).Unix(),
		MaxUses:   maxUses,
	})
	if err != nil {
		return "", nil, err
	}

	cleanup := func() {
		_, _ = client.request(passphraseAgentRequest{
			Action: agentActionClearAskPass,
			Token:  token,
		})
	}
	return token, cleanup, nil
}

func resolveAskPassTokenSecret(socketPath, token string) (string, error) {
	client := passphraseAgentClient{socketPath: socketPath}
	resp, err := client.request(passphraseAgentRequest{
		Action: agentActionGetAskPass,
		Token:  token,
	})
	if err != nil {
		return "", err
	}
	if !resp.Found {
		return "", errors.New("askpass token not found or expired")
	}
	secret, err := base64.StdEncoding.DecodeString(resp.SecretB64)
	if err != nil || len(secret) == 0 {
		return "", errors.New("invalid askpass secret payload")
	}
	return string(secret), nil
}

func newAskPassToken() (string, error) {
	buf := make([]byte, askPassTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate askpass token: %w", err)
	}
	return hex.EncodeToString(buf), nil
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
	return filepath.Join(homeDir, ".config", "onessh", "agent.sock"), nil
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
