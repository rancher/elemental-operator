/*
Copyright © 2022 - 2023 SUSE LLC

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
	"github.com/sanity-io/litter"
	"gopkg.in/yaml.v1"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/dmidecode"
	"github.com/rancher/elemental-operator/pkg/hostinfo"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/tpm"
)

type authClient interface {
	Init(reg elementalv1.Registration) error
	GetName() string
	GetToken() (string, error)
	GetPubHash() (string, error)
	Authenticate(conn *websocket.Conn) error
}

func Register(reg elementalv1.Registration, caCert []byte) ([]byte, error) {
	// add here alternate auth methods that implement the authClient interface
	var auth authClient = &tpm.AuthClient{}

	if err := auth.Init(reg); err != nil {
		return nil, fmt.Errorf("init %s authentication: %w", auth.GetName(), err)
	}

	log.Infof("Connect to %s", reg.URL)

	conn, err := initWebsocketConn(reg.URL, caCert, auth)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := authenticate(conn, auth); err != nil {
		return nil, fmt.Errorf("%s authentication failed: %w", auth.GetName(), err)
	}
	log.Infof("%s authentication completed", auth.GetName())

	log.Debugf("elemental-register protocol version: %d", MsgLast)
	protoVersion, err := negotiateProtoVersion(conn)
	if err != nil {
		if protoVersion == MsgUndefined {
			return nil, fmt.Errorf("failed to negotiate protocol version: %w", err)
		}
		// It could be an old operator which doesn't negotiate protocol version and tears down the
		// connection: give it another chance without negotiating protocol version
		conn.Close()
		time.Sleep(time.Second)
		log.Debugf("Failed to negotiate protocol version: %s. Retry connection assuming old operator.", err.Error())
		conn, err = initWebsocketConn(reg.URL, caCert, auth)
		if err != nil {
			return nil, err
		}
		if err := authenticate(conn, auth); err != nil {
			return nil, fmt.Errorf("%s authentication failed: %w", auth.GetName(), err)
		}
	}
	log.Infof("Negotiated protocol version: %d", protoVersion)

	if !reg.NoSMBIOS {
		log.Infof("Send SMBIOS data")
		if err := sendSMBIOSData(conn); err != nil {
			return nil, fmt.Errorf("failed to send SMBIOS data: %w", err)
		}

		if protoVersion >= MsgSystemData {
			log.Infof("Send system data")
			if err := sendSystemData(conn); err != nil {
				return nil, fmt.Errorf("failed to send system data: %w", err)
			}
		}
	}

	if protoVersion >= MsgAnnotations {
		log.Info("Send elemental annotations")
		if err := sendAnnotations(conn); err != nil {
			return nil, fmt.Errorf("failend to send dynamic data: %w", err)
		}
	}

	log.Info("Get elemental configuration")
	if err := WriteMessage(conn, MsgGet, []byte{}); err != nil {
		return nil, fmt.Errorf("request elemental configuration: %w", err)
	}

	if protoVersion >= MsgConfig {
		msgType, data, err := ReadMessage(conn)
		if err != nil {
			return nil, fmt.Errorf("read configuration response: %w", err)
		}

		log.Debugf("Got configuration response: %s", msgType.String())

		switch msgType {
		case MsgError:
			msg := &ErrorMessage{}
			if err = yaml.Unmarshal(data, &msg); err != nil {
				return nil, errors.Wrap(err, "unable to unmarshal error-message")
			}

			return nil, errors.New(msg.Message)

		case MsgConfig:
			return data, nil

		default:
			return nil, fmt.Errorf("unexpected response message: %s", msgType)
		}
	}

	_, r, err := conn.NextReader()
	if err != nil {
		return nil, fmt.Errorf("read elemental configuration: %w", err)
	}
	return io.ReadAll(r)
}

func initWebsocketConn(url string, caCert []byte, auth authClient) (*websocket.Conn, error) {
	dialer := websocket.DefaultDialer

	if len(caCert) > 0 {
		pool, err := x509.SystemCertPool()
		if err != nil {
			return nil, err
		}
		pool.AppendCertsFromPEM(caCert)
		dialer.TLSClientConfig = &tls.Config{RootCAs: pool}
	}

	authToken, err := auth.GetToken()
	if err != nil {
		return nil, fmt.Errorf("cannot generate authentication token: %w", err)
	}
	authHash, err := auth.GetPubHash()
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve authentication hash: %w", err)
	}

	wsURL := strings.Replace(url, "http", "ws", 1)
	log.Infof("Using %s Auth with Hash %s to dial %s", auth.GetName(), authHash, wsURL)

	header := http.Header{}
	header.Add("Authorization", authToken)

	conn, resp, err := dialer.Dial(wsURL, header)
	log.Infof("Local Address: %s", conn.LocalAddr().String())
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

func authenticate(conn *websocket.Conn, auth authClient) error {
	log.Debugf("Start %s authentication", auth.GetName())

	if err := auth.Authenticate(conn); err != nil {
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
		// This may be an old operator tearing down the connection because doesn't know MsgVersion:
		// let's report that by returning the base and oldest protocol version, i.e, MsgGet
		return MsgGet, fmt.Errorf("communication error: %w", err)
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
		log.Debugf("SMBIOS data:\n%s", litter.Sdump(data))
		return err
	}
	return nil
}

func sendSystemData(conn *websocket.Conn) error {
	data, err := hostinfo.Host()
	if err != nil {
		return errors.Wrap(err, "failed to read system data")
	}
	err = SendJSONData(conn, MsgSystemData, data)
	if err != nil {
		log.Debugf("system data:\n%s", litter.Sdump(data))
		return err
	}
	return nil
}

func sendAnnotations(conn *websocket.Conn) error {
	data := map[string]string{}
	tcpAddr := conn.LocalAddr().String()
	idxPortNumStart := strings.LastIndexAny(tcpAddr, ":")
	if idxPortNumStart < 0 {
		log.Errorf("Cannot understand local IP address format [%s], skip it", tcpAddr)
	} else {
		data["registration-ip"] = tcpAddr[0:idxPortNumStart]
		log.Debugf("sending local IP: %s", data["registration-ip"])
	}
	err := SendJSONData(conn, MsgAnnotations, data)
	if err != nil {
		log.Debugf("annotation data:\n%s", litter.Sdump(data))
		return err
	}
	return nil
}
