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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/rancher/elemental-operator/pkg/dmidecode"
	"github.com/rancher/elemental-operator/pkg/tpm"
	"github.com/sanity-io/litter"
	"github.com/sirupsen/logrus"
)

func Register(url string, caCert []byte, smbios bool, emulatedTPM bool, emulatedSeed int64) ([]byte, error) {

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

	conn, resp, err := dialer.Dial(wsURL, header)
	if err != nil {
		if resp != nil {
			if resp.StatusCode == http.StatusUnauthorized {
				data, err := io.ReadAll(resp.Body)
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

	logrus.Debug("start TPM attestation")
	err = tpmAuth.Init(conn)
	if err != nil {
		return nil, fmt.Errorf("failed TPM based authentication: %w", err)
	}

	msgType, _, err := ReadMessage(conn)
	if err != nil {
		return nil, fmt.Errorf("expecting auth reply: %w", err)
	}
	if msgType != MsgReady {
		return nil, fmt.Errorf("expecting '%v' but got '%v'", MsgReady, msgType)
	}
	logrus.Info("TPM attestation successful")

	// Send the version of the communication protocol we support. Old operator (before kubebuilder rework)
	// will not even reply and will tear down the connection.
	logrus.Infof("elemental-register protocol: %d", MsgLast)
	data := []byte{byte(MsgLast)}
	if err = WriteMessage(conn, MsgVersion, data); err != nil {
		return nil, err
	}

	// Retrieve the version of the communication protocol supported by the operator. This could be of help
	// to decide what we can 'ask' to the operator in future releases (we don't really do nothing with this
	// right now).
	msgType, data, err = ReadMessage(conn)
	if err == nil && msgType != MsgVersion {
		err = fmt.Errorf("expected msg %s, got %s", MsgVersion, msgType)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to negotiate the protocol version: %w", err)
	}
	if len(data) != 1 {
		return nil, fmt.Errorf("failed to decode protocol version, got %v (%s)", data, data)
	}
	protoVersion := MessageType(data[0])
	logrus.Infof("elemental-operator protocol: %d", protoVersion)

	if smbios {
		if err := sendSMBIOSData(conn); err != nil {
			return nil, err
		}
	}

	logrus.Debug("get elemental configuration")
	if err := WriteMessage(conn, MsgGet, []byte{}); err != nil {
		return nil, fmt.Errorf("request elemental configuration: %w", err)
	}
	_, r, err := conn.NextReader()
	if err != nil {
		return nil, fmt.Errorf("read elemental configuration: %w", err)
	}
	return io.ReadAll(r)
}

func sendSMBIOSData(conn *websocket.Conn) error {
	logrus.Debug("send SMBIOS data")
	data, err := dmidecode.Decode()
	if err != nil {
		return errors.Wrap(err, "failed to read dmidecode data")
	}
	err = SendJSONData(conn, MsgSmbios, data)
	if err != nil {
		logrus.Debugf("SMBIOS data:\n%s", litter.Sdump(data))
		return fmt.Errorf("sending SMBIOS data: %w", err)
	}
	return nil
}
