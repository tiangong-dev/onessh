package cli

import (
	"io"

	"github.com/tiangong-dev/shush"
)

func servePassphraseAgent(socketPath string, errOut io.Writer) error {
	return shush.Serve(socketPath, errOut)
}

func servePassphraseAgentWithCapability(socketPath string, errOut io.Writer, capability string) error {
	return shush.ServeWithCapability(socketPath, errOut, resolveAgentCapability(capability))
}
