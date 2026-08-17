//go:build !linux

package egress

import "syscall"

func enableIPv6FreeBind(syscall.RawConn) error {
	return ErrIPv6Unsupported
}
