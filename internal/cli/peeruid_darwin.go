//go:build darwin

package cli

import (
	"fmt"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

func socketPeerUID(conn net.Conn) (uint32, error) {
	sysConn, ok := conn.(syscall.Conn)
	if !ok {
		return 0, fmt.Errorf("connection does not expose syscall.Conn")
	}

	rawConn, err := sysConn.SyscallConn()
	if err != nil {
		return 0, err
	}

	var uid uint32
	var controlErr error
	if err := rawConn.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if err != nil {
			controlErr = err
			return
		}
		uid = cred.Uid
	}); err != nil {
		return 0, err
	}
	if controlErr != nil {
		return 0, controlErr
	}
	return uid, nil
}
