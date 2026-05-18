package ssh

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"onessh/internal/domain"
	"onessh/internal/store"
)

// ArgsOptions contains dependencies needed while constructing SSH arguments.
type ArgsOptions struct {
	OnesshPath        string
	ResolveOnesshPath func() (string, error)
}

// Destination builds the ssh/scp destination from the host and resolved user.
func Destination(host store.HostConfig, userName string) string {
	destination := host.Host
	if userName != "" {
		destination = fmt.Sprintf("%s@%s", userName, host.Host)
	}
	return destination
}

// BuildSSHFlags builds the SSH option flags without the destination.
func BuildSSHFlags(cfg store.PlainConfig, host store.HostConfig, auth store.AuthConfig, extra []string, opts ArgsOptions) ([]string, error) {
	args, err := applySSHCommonArgs(nil, cfg, host, "-p", opts)
	if err != nil {
		return nil, err
	}
	args = appendSendEnvOptions(args, host.Env)
	args, err = applyKeyAuthArg(args, auth)
	if err != nil {
		return nil, err
	}
	args = append(args, extra...)
	return args, nil
}

// BuildSSHArgs builds the full SSH argument list including the destination.
func BuildSSHArgs(cfg store.PlainConfig, host store.HostConfig, userName string, auth store.AuthConfig, extra []string, opts ArgsOptions) ([]string, error) {
	args, err := BuildSSHFlags(cfg, host, auth, extra, opts)
	if err != nil {
		return nil, err
	}
	args = append(args, Destination(host, userName))
	return args, nil
}

// BuildSCPArgs builds the full SCP argument list for uploads and downloads.
func BuildSCPArgs(cfg store.PlainConfig, host store.HostConfig, userName string, auth store.AuthConfig, remotePath string, localPaths []string, isUpload, recursive bool, opts ArgsOptions) ([]string, error) {
	args, err := applySSHCommonArgs(nil, cfg, host, "-P", opts)
	if err != nil {
		return nil, err
	}
	if recursive {
		args = append(args, "-r")
	}
	args, err = applyKeyAuthArg(args, auth)
	if err != nil {
		return nil, err
	}

	remote := Destination(host, userName) + ":" + remotePath
	if isUpload {
		args = append(args, localPaths...)
		args = append(args, remote)
		return args, nil
	}
	args = append(args, remote, localPaths[0])
	return args, nil
}

func (opts ArgsOptions) resolveOnesshPath() (string, error) {
	if strings.TrimSpace(opts.OnesshPath) != "" {
		return opts.OnesshPath, nil
	}
	if opts.ResolveOnesshPath != nil {
		return opts.ResolveOnesshPath()
	}
	return "", errors.New("onessh path is required for password proxy jump")
}

func applySSHCommonArgs(args []string, cfg store.PlainConfig, host store.HostConfig, portFlag string, opts ArgsOptions) ([]string, error) {
	port := domain.EffectivePort(host.Port)
	args = append(args, portFlag, strconv.Itoa(port))
	if host.ProxyJump != "" {
		proxyArgs, err := BuildProxyJumpArgs(cfg, host.ProxyJump, opts)
		if err != nil {
			return nil, err
		}
		args = append(args, proxyArgs...)
	}
	return args, nil
}

func applyKeyAuthArg(args []string, auth store.AuthConfig) ([]string, error) {
	switch domain.NormalizeAuthType(auth.Type) {
	case domain.AuthTypeKey:
		if auth.KeyPath == "" {
			return args, nil
		}
		keyPath, err := expandTilde(auth.KeyPath)
		if err != nil {
			return nil, err
		}
		return append(args, "-i", keyPath), nil
	case domain.AuthTypePassword:
		return args, nil
	default:
		return nil, fmt.Errorf("unsupported auth type: %s", auth.Type)
	}
}

func appendSendEnvOptions(args []string, envMap map[string]string) []string {
	if len(envMap) == 0 {
		return args
	}
	keys := sortedStringMapKeys(envMap)
	for _, key := range keys {
		args = append(args, "-o", "SendEnv="+key)
	}
	return args
}

func sortedStringMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
