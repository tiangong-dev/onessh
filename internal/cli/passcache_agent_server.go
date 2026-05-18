package cli

import (
	"io"

	infraagent "onessh/internal/infra/agent"
)

func servePassphraseAgent(socketPath string, errOut io.Writer) error {
	return infraagent.Serve(socketPath, errOut)
}

func servePassphraseAgentWithCapability(socketPath string, errOut io.Writer, capability string) error {
	return infraagent.ServeWithCapability(socketPath, errOut, capability)
}
