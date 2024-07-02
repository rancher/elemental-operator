/*
Copyright © 2022 - 2024 SUSE LLC

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
	"encoding/json"
	"fmt"
	"io"

	"github.com/gorilla/websocket"
)

const RegistrationDeadlineSeconds = 10

type MessageType byte

const (
	MsgUndefined MessageType = iota
	MsgReady
	MsgSmbios
	MsgLabels
	MsgGet                              // v0.5.0
	MsgVersion                          // v1.1.0
	MsgSystemData                       // v1.1.1
	MsgConfig                           // v1.1.1
	MsgError                            // v1.1.1
	MsgAnnotations                      // v1.1.4
	MsgUpdate                           // v1.2.6
	MsgSystemDataV2                     // v1.6.0
	MsgNetworkConfig                    // v1.7.0
	MsgLast          = MsgNetworkConfig // MsgLast must point to the last message
)

func (mt MessageType) String() string {
	switch mt {
	case MsgUndefined:
		return "Undefined"
	case MsgReady:
		return "Ready"
	case MsgSmbios:
		return "SMBIOS"
	case MsgLabels:
		return "Labels"
	case MsgGet:
		return "Get"
	case MsgVersion:
		return "Version"
	case MsgSystemData:
		return "System"
	case MsgConfig:
		return "Config"
	case MsgError:
		return "Error"
	case MsgAnnotations:
		return "Annotations"
	case MsgUpdate:
		return "Update"
	case MsgSystemDataV2:
		return "SystemDataV2"
	case MsgNetworkConfig:
		return "NetworkConfig"
	default:
		return "Unknown"
	}
}

type ErrorMessage struct {
	Message string `json:"message,omitempty" yaml:"message"`
}

func NewErrorMessage(err error) ErrorMessage {
	return ErrorMessage{
		Message: err.Error(),
	}
}

// ReadMessage reads from the websocket connection returning the MessageType
// and the actual data
func ReadMessage(conn *websocket.Conn) (MessageType, []byte, error) {
	msgType := MsgUndefined

	t, r, err := conn.NextReader()
	if err != nil {
		return msgType, nil, err
	}
	if t != websocket.BinaryMessage {
		return msgType, nil, fmt.Errorf("got non binary message (type %d)", t)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return msgType, nil, err
	}
	return extractMessageType(data)
}

// WriteMessage attaches msgType to the actual data before sending it to the
// websocket connection
func WriteMessage(conn *websocket.Conn, msgType MessageType, data []byte) error {
	w, err := conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return err
	}
	defer w.Close()

	buf := insertMessageType(msgType, data)
	_, err = w.Write(buf)
	return err
}

// SendJSONData transmits json encoded data to the websocket connection, attaching to
// it msgType. It needs a `MsgReady` reply on the websocket channel in order to report
// a successful transmission
func SendJSONData(conn *websocket.Conn, msgType MessageType, data interface{}) error {
	var buf []byte

	w, err := conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return err
	}
	_, err = w.Write(insertMessageType(msgType, buf))
	if err != nil {
		return err
	}
	if err = json.NewEncoder(w).Encode(data); err != nil {
		return err
	}
	w.Close()

	mt, _, err := ReadMessage(conn)
	if err != nil {
		return err
	}
	if mt != MsgReady {
		return fmt.Errorf("expecting '%v' but got '%v'", MsgReady, mt)
	}

	return nil
}

func extractMessageType(buf []byte) (MessageType, []byte, error) {
	if len(buf) < 1 {
		return MsgUndefined, buf, fmt.Errorf("empty message")
	}
	if buf[0] > byte(MsgLast) {
		return MsgUndefined, buf, fmt.Errorf("unknown message")
	}

	return MessageType(buf[0]), buf[1:], nil
}

func insertMessageType(msg MessageType, buf []byte) []byte {
	msgBuf := []byte{byte(msg)}
	return append(msgBuf, buf...)
}
