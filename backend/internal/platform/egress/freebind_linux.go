//go:build linux

package egress

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func enableIPv6FreeBind(raw syscall.RawConn) error {
	var controlErr error
	if err := raw.Control(func(fd uintptr) {
		controlErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_FREEBIND, 1)
	}); err != nil {
		return err
	}
	return controlErr
}
