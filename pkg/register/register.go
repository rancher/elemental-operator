/*
Copyright © 2022 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package register

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/rancher/elemental-operator/pkg/dmidecode"
	"github.com/rancher/elemental-operator/pkg/tpm"
	"github.com/sirupsen/logrus"
)

func Register(url string, caCert []byte, smbios bool, emulatedTPM bool, emulatedSeed int64, labels map[string]string) ([]byte, error) {

	dialer := websocket.DefaultDialer

	if len(caCert) > 0 {
		pool, err := x509.SystemCertPool()
		if err != nil {
			return nil, err
		}
		pool.AppendCertsFromPEM(caCert)
		dialer.TLSClientConfig = &tls.Config{RootCAs: pool}
	}

	tpmAuth := &tpm.AuthClient{}
	if emulatedTPM {
		logrus.Info("Starting TPM emulation")
		tpmAuth.EmulateTPM(emulatedSeed)
	}
	authToken, tpmHash, err := tpmAuth.GetAuthToken()
	if err != nil {
		return nil, fmt.Errorf("cannot generate authentication token from TPM: %w", err)
	}

	wsURL := strings.Replace(url, "http", "ws", 1)
	logrus.Infof("Using TPMHash %s to dial %s", tpmHash, wsURL)

	header := http.Header{}
	header.Add("Authorization", authToken)
	if smbios {
		err = addSMBIOSHeaders(&header)
		if err != nil {
			return nil, fmt.Errorf("add SMBIOS headers: %w", err)
		}
	}
	if len(labels) > 0 {
		err = addLabelsHeaders(&header, labels)
		if err != nil {
			return nil, fmt.Errorf("add labels in header: %w", err)
		}
	}

	conn, resp, err := dialer.Dial(wsURL, header)
	if err != nil {
		if resp != nil {
			if resp.StatusCode == http.StatusUnauthorized {
				data, err := ioutil.ReadAll(resp.Body)
				if err == nil {
					return nil, errors.New(string(data))
				}
			} else {
				return nil, fmt.Errorf("%w (Status: %s)", err, resp.Status)
			}
		}
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(RegistrationDeadlineSeconds * time.Second))
	_ = conn.SetReadDeadline(time.Now().Add(RegistrationDeadlineSeconds * time.Second))

	err = tpmAuth.Init(conn)
	if err != nil {
		return nil, fmt.Errorf("failed TPM based authentication: %w", err)
	}

	_, msg, err := conn.NextReader()
	if err != nil {
		return nil, fmt.Errorf("reading elemental config: %w", err)
	}

	return ioutil.ReadAll(msg)
}

func addSMBIOSHeaders(header *http.Header) error {
	data, err := dmidecode.Decode()
	if err != nil {
		return errors.Wrap(err, "failed to read dmidecode data")
	}

	var buf bytes.Buffer
	b64Enc := base64.NewEncoder(base64.StdEncoding, &buf)

	if err = json.NewEncoder(b64Enc).Encode(data); err != nil {
		return errors.Wrap(err, "failed to encode dmidecode data")
	}

	_ = b64Enc.Close()

	chunk := make([]byte, 875) //the chunk size
	part := 1
	for {
		n, err := buf.Read(chunk)
		if err != nil {
			if err != io.EOF {
				return errors.Wrap(err, "failed to read file in chunks")
			}
			break
		}
		header.Set(fmt.Sprintf("X-Cattle-Smbios-%d", part), string(chunk[0:n]))
		part++
	}
	return nil
}

func addLabelsHeaders(header *http.Header, labels map[string]string) error {
	var buf bytes.Buffer
	b64Enc := base64.NewEncoder(base64.StdEncoding, &buf)

	if err := json.NewEncoder(b64Enc).Encode(labels); err != nil {
		return errors.Wrap(err, "failed to encode labels")
	}

	_ = b64Enc.Close()
	header.Set("X-Cattle-Labels", buf.String())
	return nil
}
