package tpm

import (
	"hash/fnv"
	"net/http"
	"os"

	"github.com/google/go-attestation/attest"
)

type config struct {
	emulated       bool
	commandChannel attest.CommandChannelTPM20
	seed           int64

	cacerts []byte
	header  http.Header

	systemfallback bool
}

// Option is a generic option for TPM configuration
type Option func(c *config) error

// Emulated sets an emulated device in place of a real native TPM device.
// Note, the emulated device is embedded and it is unsafe.
// Should just be used for testing.
var Emulated Option = func(c *config) error {
	c.emulated = true
	return nil
}

// AppendCustomCAToSystemCA uses the system CA pool as a fallback, appending the custom CA
// to it.
var AppendCustomCAToSystemCA Option = func(c *config) error {
	c.systemfallback = true
	return nil
}

// WithCAs sets the root CAs for the request
func WithCAs(ca []byte) Option {
	return func(c *config) error {
		c.cacerts = ca
		return nil
	}
}

// WithHeader sets a specific header for the request
func WithHeader(header http.Header) Option {
	return func(c *config) error {
		c.header = header
		return nil
	}
}

// WithSeed sets a permanent seed. Used with TPM emulated device.
func WithSeed(s int64) Option {
	return func(c *config) error {
		c.seed = s
		return nil
	}
}

// WithCommandChannel overrides the TPM command channel
func WithCommandChannel(cc attest.CommandChannelTPM20) Option {
	return func(c *config) error {
		c.commandChannel = cc
		return nil
	}
}

func hashit(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func tokenize() int64 {
	hostname, err := os.Hostname()
	if err != nil {
		return 0
	}
	return int64(hashit(hostname))
}

// EmulatedHostSeed generates a seed based on the hostname
var EmulatedHostSeed = func() Option {
	return func(c *config) error {
		c.seed = tokenize()
		return nil
	}
}

func (c *config) apply(opts ...Option) error {
	for _, o := range opts {
		if err := o(c); err != nil {
			return err
		}
	}

	return nil
}
