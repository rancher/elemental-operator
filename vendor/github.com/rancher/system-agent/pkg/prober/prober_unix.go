//go:build !windows
// +build !windows

package prober

import (
	"crypto/x509"
	"fmt"

	"github.com/sirupsen/logrus"
)

// GetSystemCertPool returns a x509.CertPool that contains the
// root CA certificates if they are present at runtime
func GetSystemCertPool(probeName string) (*x509.CertPool, error) {
	caCertPool, err := x509.SystemCertPool()
	if err != nil {
		caCertPool = x509.NewCertPool()
		logrus.Errorf("[GetSystemCertPoolUnix] error loading system cert pool for probe (%s): %v", probeName, err)
	}
	if caCertPool == nil {
		return nil, fmt.Errorf("[GetSystemCertPoolWindows] x509 returned a nil certpool for probe (%s)", probeName)
	}
	return caCertPool, nil
}
