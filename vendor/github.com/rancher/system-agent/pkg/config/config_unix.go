//go:build !windows
// +build !windows

package config

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// pathOwnedByRoot is abstracted out for Linux as root is a Linux only concept.
func pathOwnedByRoot(fi os.FileInfo, path string) error {
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("failed type assertion for *syscall.Stat_t")
	}
	if stat.Uid != 0 || stat.Gid != 0 {
		return fmt.Errorf("file %s had was not owned by root:root", path)
	}
	return nil
}

// permissionsCheck is abstracted out for Linux as root is a Linux only concept.
func permissionsCheck(fi os.FileInfo, path string) error {
	if fi.Mode().Perm() != 0600 {
		return fmt.Errorf("file %s had permission %#o which was not expected 0600", path, fi.Mode().Perm())
	}
	return nil
}
