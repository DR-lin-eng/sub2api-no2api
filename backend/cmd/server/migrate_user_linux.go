//go:build linux

package main

import (
	"errors"
	"os"
	"syscall"
)

// docker exec bypasses the image's entrypoint and otherwise leaves private
// checkpoints/reports owned by root, inaccessible to the running application.
func migrationImageUser() error {
	if os.Geteuid() != 0 {
		return nil
	}
	info, err := os.Lstat("/app/docker-entrypoint.sh")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("invalid container entrypoint")
	}
	if _, err := os.Stat("/.dockerenv"); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec("/sbin/su-exec", append([]string{"su-exec", "sub2api", executable}, os.Args[1:]...), os.Environ())
}
