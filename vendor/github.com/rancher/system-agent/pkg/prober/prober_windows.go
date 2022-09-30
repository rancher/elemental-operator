//go:build windows
// +build windows

package prober

import (
	"crypto/x509"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/sirupsen/logrus"
)

const (
	CRYPT_E_NOT_FOUND = 0x80092004
	maxEncodedCertLen = 1 << 20
)

// GetSystemCertPool is a workaround to Windows not having x509.SystemCertPool implemented in < go1.18
// it leverages syscalls to extract system certificates and load them into a new x509.CertPool
// workaround adapted from: https://github.com/golang/go/issues/16736#issuecomment-540373689
// ref: https://docs.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certgetissuercertificatefromstore
// TODO: Test and remove after system-agent is bumped to go1.18+
func GetSystemCertPool(probeName string) (*x509.CertPool, error) {
	logrus.Tracef("[GetSystemCertPoolWindows] building system certContext pool for probe (%s)", probeName)
	root, err := syscall.UTF16PtrFromString("Root")
	if err != nil {
		return nil, fmt.Errorf("[GetSystemCertPoolWindows] unable to return UTF16 pointer: %v", syscall.GetLastError())
	}
	if root == nil {
		return nil, fmt.Errorf("[GetSystemCertPoolWindows] UTF16 pointer for Root returned nil: %v", syscall.GetLastError())

	}
	// win32 reference: https://docs.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certopensystemstorea
	// If the function succeeds, it returns a handle to the specified certificate store.
	storeHandle, err := syscall.CertOpenSystemStore(0, root)
	if err != nil {
		return nil, fmt.Errorf("[GetSystemCertPoolWindows] unable to open system certContext store: %v", syscall.GetLastError())
	}

	var certs []*x509.Certificate
	var certContext *syscall.CertContext

	// ref for why flags value is 0: https://docs.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certclosestore
	defer func() {
		if certContext != nil {
			_ = syscall.CertCloseStore(certContext.Store, 0)
		}
	}()

	// this for loop will iterate through all available certificates in the specified certificate store
	// and build an array of each x509.Certificate that is returned
	for {
		// CertEnumCertificatesInStore returns a single certContext containing the initial/next certificate in the cert store
		certContext, err = syscall.CertEnumCertificatesInStore(storeHandle, certContext)
		if err != nil {
			if errno, ok := err.(syscall.Errno); ok {
				if errno == CRYPT_E_NOT_FOUND {
					// if the error returned here is CRYPTO_E_NOT_FOUND, that indicates no certificates were found.
					// This happens if the store is empty or if the function reached the end of the store's list.
					// ref: https://docs.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certenumcertificatesinstore#return-value
					logrus.Debugf("[GetSystemCertPoolWindows] no certificates were returned from the root CA store for probe (%s)", probeName)
					break
				}
			}
			logrus.Errorf("[GetSystemCertPoolWindows] unable to enumerate certs in system certContext store for probe (%s): %v", probeName, syscall.GetLastError())
		}
		if certContext == nil {
			logrus.Errorf("[GetSystemCertPoolWindows] certificate context returned from syscall is nil for probe (%s)", probeName)
			break
		}

		// buf is a ~1048 kilobyte array that serves as a buffer holding the encoded value
		// of a single CA certificate returned from the Windows root CA store
		// equal to the length of the certContext pointer which contains a certificate from the Root CA store
		//
		// we are sizing for a single context (certificate) and not the whole store in buf
		// using a binary shift to create a ~1048 Kb buffer (slightly larger than 1 megabyte)
		// [1 << 20]byte -> (1*2)^20 = 1048576 bytes
		// stating for reference but not related to this code
		// the maximum size of a Windows certificate store is 16 kilobytes and is not related to number of certificates
		if certContext.Length > maxEncodedCertLen {
			return nil, fmt.Errorf("invalid CertContext length %d", certContext.Length)
		}
		buf := (*[maxEncodedCertLen]byte)(unsafe.Pointer(certContext.EncodedCert))[:certContext.Length]

		// validate the root CA certificate and return a x509.Certificate pointer
		// that is appended into our array of x509 certificates
		c, err := x509.ParseCertificate(buf)
		if err != nil {
			return nil, fmt.Errorf("[GetSystemCertPoolWindows] unable to parse x509 certificate for probe (%s): %v", probeName, err)
		}
		certs = append(certs, c)
		logrus.Debugf("[GetSystemCertPoolWindows] Successfully loaded %d certificates from system certContext store for probe (%s)", len(certs), probeName)
	}

	caCertPool := x509.NewCertPool()
	if caCertPool == nil {
		return nil, fmt.Errorf("[GetSystemCertPoolWindows] x509 returned a nil certpool for probe (%s)", probeName)
	}

	for _, certificate := range certs {
		if !caCertPool.AppendCertsFromPEM(certificate.RawTBSCertificate) {
			return nil, fmt.Errorf("[GetSystemCertPoolWindows] unable to append certContext with CN [%s] to system certContext pool for probe (%s)", certificate.Subject.CommonName, probeName)
		}
		logrus.Tracef("[GetSystemCertPoolWindows] successfully appended certContext with CN [%s] to system certContext pool for probe (%s)", certificate.Subject.CommonName, probeName)
	}
	logrus.Infof("[GetSystemCertPoolWindows] Successfully loaded %d certificates into system certContext pool for probe (%s)", len(certs), probeName)

	return caCertPool, nil
}
