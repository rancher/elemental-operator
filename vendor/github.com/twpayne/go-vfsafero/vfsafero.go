// Package vfsafero provides a compatibility later between
// github.com/twpayne/go-vfs and github.com/spf13/afero.
package vfsafero

import (
	"os"

	"github.com/spf13/afero"
	vfs "github.com/twpayne/go-vfs"
)

// An AferoFS implements github.com/spf13/afero.Fs.
type AferoFS struct {
	vfs.FS
}

// NewAferoFS returns a new AferoFS.
func NewAferoFS(fs vfs.FS) *AferoFS {
	return &AferoFS{
		FS: fs,
	}
}

// Create implements afero.Fs.Create.
func (a *AferoFS) Create(name string) (afero.File, error) {
	return a.FS.Create(name)
}

// LstatIfPossible implements afero.Lstater.LstatIfPossible.
func (a *AferoFS) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	info, err := a.Lstat(name)
	return info, true, err
}

// MkdirAll implements afero.Fs.MkdirAll.
func (a *AferoFS) MkdirAll(path string, perm os.FileMode) error {
	return vfs.MkdirAll(a.FS, path, perm)
}

// Name implements afero.Fs.Name.
func (a *AferoFS) Name() string {
	return "AferoFS"
}

// Open implements afero.Fs.Open.
func (a *AferoFS) Open(name string) (afero.File, error) {
	return a.FS.Open(name)
}

// OpenFile implements afero.Fs.OpenFile.
func (a *AferoFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	return a.FS.OpenFile(name, flag, perm)
}
