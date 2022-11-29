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

func Register(url string, caCert []byte, smbios bool, emulateTPM bool, emulatedSeed int64) ([]byte, error) {

	tpmAuth := &tpm.AuthClient{}
	if emulateTPM {
		logrus.Info("Enable TPM emulation")
		tpmAuth.EmulateTPM(emulatedSeed)
	}

	logrus.Infof("Connect to %s", url)
	conn, err := initWebsocketConn(url, caCert, tpmAuth)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	logrus.Debug("Start TPM attestation")
	if err := doTPMAttestation(tpmAuth, conn); err != nil {
		return nil, fmt.Errorf("failed TPM based auth: %w", err)
	}
	logrus.Info("TPM attestation successful")

	logrus.Debugf("elemental-register protocol version: %d", MsgLast)
	protoVersion, err := negotiateProtoVersion(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to negotiate protocol version: %w", err)
	}
	logrus.Infof("Negotiated protocol version: %d", protoVersion)

	if smbios {
		logrus.Info("Send SMBIOS data")
		if err := sendSMBIOSData(conn); err != nil {
			return nil, fmt.Errorf("failed to send SMBIOS data: %w", err)
		}
	}

	logrus.Info("Get elemental configuration")
	if err := WriteMessage(conn, MsgGet, []byte{}); err != nil {
		return nil, fmt.Errorf("request elemental configuration: %w", err)
	}
	_, r, err := conn.NextReader()
	if err != nil {
		return nil, fmt.Errorf("read elemental configuration: %w", err)
	}
	return io.ReadAll(r)
}

func initWebsocketConn(url string, caCert []byte, tpmAuth *tpm.AuthClient) (*websocket.Conn, error) {
	dialer := websocket.DefaultDialer

	if len(caCert) > 0 {
		pool, err := x509.SystemCertPool()
		if err != nil {
			return nil, err
		}
		pool.AppendCertsFromPEM(caCert)
		dialer.TLSClientConfig = &tls.Config{RootCAs: pool}
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
	_ = conn.SetWriteDeadline(time.Now().Add(RegistrationDeadlineSeconds * time.Second))
	_ = conn.SetReadDeadline(time.Now().Add(RegistrationDeadlineSeconds * time.Second))

	return conn, nil
}

func doTPMAttestation(tpmAuth *tpm.AuthClient, conn *websocket.Conn) error {
	if err := tpmAuth.Init(conn); err != nil {
		return err
	}

	msgType, _, err := ReadMessage(conn)
	if err != nil {
		return fmt.Errorf("expecting auth reply: %w", err)
	}
	if msgType != MsgReady {
		return fmt.Errorf("expecting '%v' but got '%v'", MsgReady, msgType)
	}
	return nil
}

func negotiateProtoVersion(conn *websocket.Conn) (MessageType, error) {
	// Send the version of the communication protocol we support. Old operator (before kubebuilder rework)
	// will not even reply and will tear down the connection.
	data := []byte{byte(MsgLast)}
	if err := WriteMessage(conn, MsgVersion, data); err != nil {
		return MsgUndefined, err
	}

	// Retrieve the version of the communication protocol supported by the operator. This could be of help
	// to decide what we can 'ask' to the operator in future releases (we don't really do nothing with this
	// right now).
	msgType, data, err := ReadMessage(conn)
	if err != nil {
		return MsgUndefined, fmt.Errorf("communication error: %w", err)
	}
	if msgType != MsgVersion {
		return MsgUndefined, fmt.Errorf("expected msg %s, got %s", MsgVersion, msgType)
	}
	if len(data) != 1 {
		return MsgUndefined, fmt.Errorf("failed to decode protocol version, got %v (%s)", data, data)
	}
	return MessageType(data[0]), err
}

func sendSMBIOSData(conn *websocket.Conn) error {
	data, err := dmidecode.Decode()
	if err != nil {
		return errors.Wrap(err, "failed to read dmidecode data")
	}
	err = SendJSONData(conn, MsgSmbios, data)
	if err != nil {
		logrus.Debugf("SMBIOS data:\n%s", litter.Sdump(data))
		return err
	}
	return nil
}
