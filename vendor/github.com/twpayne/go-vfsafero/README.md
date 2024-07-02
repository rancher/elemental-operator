# go-vfsafero

[![GoDoc](https://godoc.org/github.com/twpayne/go-vfsafero?status.svg)](https://godoc.org/github.com/twpayne/go-vfsafero)
[![Build Status](https://travis-ci.org/twpayne/go-vfsafero.svg?branch=master)](https://travis-ci.org/twpayne/go-vfsafero)
[![Report Card](https://goreportcard.com/badge/github.com/twpayne/go-vfsafero)](https://goreportcard.com/report/github.com/twpayne/go-vfsafero)

Package `vfsafero` provides a compatibility later between
[`github.com/twpayne/go-vfs`](https://github.com/twpayne/go-vfs) and
[`github.com/spf13/afero`](https://github.com/spf13/afero).

This allows you to use `vfst` to test exisiting code that uses
[`afero.Fs`](https://godoc.org/github.com/spf13/afero#Fs), and use `vfs.FS`s as
`afero.Fs`s. See [the
documentation](https://godoc.org/github.com/twpayne/go-vfsafero) for an example.

## License

MIT
