//go:build windows
// +build windows

package config

import (
	"os"
)

// pathOwnedByRoot is abstracted out with Windows being a no op.
func pathOwnedByRoot(fi os.FileInfo, path string) error {
	return nil
}

// permissionsCheck is abstracted out with Windows being a no op.
func permissionsCheck(fi os.FileInfo, path string) error {
	return nil
}
