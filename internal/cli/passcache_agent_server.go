package cli

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

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
		entries:        map[string]agentEntry{},
		askpassEntries: map[string]askpassEntry{},
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
	case agentActionSetAskPass:
		return s.handleSetAskPass(req)
	case agentActionGetAskPass:
		return s.handleGetAskPass(req)
	case agentActionClearAskPass:
		return s.handleClearAskPass(req)
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
	dataPath := strings.TrimSpace(req.DataPath)
	if dataPath == "" {
		return passphraseAgentResponse{OK: false, Error: "data_path is required"}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[dataPath]
	if !ok {
		return passphraseAgentResponse{OK: true, Found: false}
	}
	if entry.expiresAt <= time.Now().Unix() {
		wipe(entry.secret)
		delete(s.entries, dataPath)
		return passphraseAgentResponse{OK: true, Found: false}
	}
	return passphraseAgentResponse{
		OK:        true,
		Found:     true,
		SecretB64: base64.StdEncoding.EncodeToString(entry.secret),
	}
}

func (s *passphraseAgentState) handleSet(req passphraseAgentRequest) passphraseAgentResponse {
	dataPath := strings.TrimSpace(req.DataPath)
	if dataPath == "" {
		return passphraseAgentResponse{OK: false, Error: "data_path is required"}
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

	if old, ok := s.entries[dataPath]; ok {
		wipe(old.secret)
	}
	s.entries[dataPath] = agentEntry{
		secret:    secret,
		expiresAt: req.ExpiresAt,
	}
	return passphraseAgentResponse{OK: true}
}

func (s *passphraseAgentState) handleClear(req passphraseAgentRequest) passphraseAgentResponse {
	dataPath := strings.TrimSpace(req.DataPath)

	s.mu.Lock()
	defer s.mu.Unlock()

	if dataPath == "" {
		for key, entry := range s.entries {
			wipe(entry.secret)
			delete(s.entries, key)
		}
		return passphraseAgentResponse{OK: true}
	}

	if entry, ok := s.entries[dataPath]; ok {
		wipe(entry.secret)
		delete(s.entries, dataPath)
	}
	return passphraseAgentResponse{OK: true}
}

func (s *passphraseAgentState) handleSetAskPass(req passphraseAgentRequest) passphraseAgentResponse {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return passphraseAgentResponse{OK: false, Error: "token is required"}
	}
	if req.ExpiresAt <= time.Now().Unix() {
		return passphraseAgentResponse{OK: false, Error: "expires_at must be in the future"}
	}
	if req.MaxUses <= 0 {
		return passphraseAgentResponse{OK: false, Error: "max_uses must be positive"}
	}

	secret, err := base64.StdEncoding.DecodeString(req.SecretB64)
	if err != nil || len(secret) == 0 {
		return passphraseAgentResponse{OK: false, Error: "invalid secret_b64"}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if old, ok := s.askpassEntries[token]; ok {
		wipe(old.secret)
	}
	s.askpassEntries[token] = askpassEntry{
		secret:    secret,
		expiresAt: req.ExpiresAt,
		remaining: req.MaxUses,
	}
	return passphraseAgentResponse{OK: true}
}

func (s *passphraseAgentState) handleGetAskPass(req passphraseAgentRequest) passphraseAgentResponse {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return passphraseAgentResponse{OK: false, Error: "token is required"}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.askpassEntries[token]
	if !ok {
		return passphraseAgentResponse{OK: true, Found: false}
	}
	if entry.expiresAt <= time.Now().Unix() || entry.remaining <= 0 {
		wipe(entry.secret)
		delete(s.askpassEntries, token)
		return passphraseAgentResponse{OK: true, Found: false}
	}

	secretCopy := append([]byte(nil), entry.secret...)
	entry.remaining--
	if entry.remaining <= 0 {
		wipe(entry.secret)
		delete(s.askpassEntries, token)
	} else {
		s.askpassEntries[token] = entry
	}

	return passphraseAgentResponse{
		OK:        true,
		Found:     true,
		SecretB64: base64.StdEncoding.EncodeToString(secretCopy),
	}
}

func (s *passphraseAgentState) handleClearAskPass(req passphraseAgentRequest) passphraseAgentResponse {
	token := strings.TrimSpace(req.Token)

	s.mu.Lock()
	defer s.mu.Unlock()

	if token == "" {
		for key, entry := range s.askpassEntries {
			wipe(entry.secret)
			delete(s.askpassEntries, key)
		}
		return passphraseAgentResponse{OK: true}
	}

	if entry, ok := s.askpassEntries[token]; ok {
		wipe(entry.secret)
		delete(s.askpassEntries, token)
	}
	return passphraseAgentResponse{OK: true}
}

func (s *passphraseAgentState) clearAll() {
	_ = s.handleClear(passphraseAgentRequest{})
	_ = s.handleClearAskPass(passphraseAgentRequest{})
}
