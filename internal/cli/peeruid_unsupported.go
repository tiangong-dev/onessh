//go:build !darwin && !linux

package cli

import (
	"errors"
	"net"
)

func socketPeerUID(_ net.Conn) (uint32, error) {
	return 0, errors.New("peer uid check is not supported on this platform")
}
