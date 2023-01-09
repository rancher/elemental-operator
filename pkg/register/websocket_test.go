/*
Copyright © SUSE LLC

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
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"gotest.tools/assert"
)

var upgrader = websocket.Upgrader{}

func getLocalAddress() string {
	l, err := net.ListenTCP("tcp", nil)
	if err != nil {
		return "localhost:8080"
	}
	defer l.Close()
	return l.Addr().String()
}

func serveEcho(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	for {
		t, r, err := conn.NextReader()
		if err != nil {
			return
		}
		// send back a MsgReady ack
		err = conn.WriteMessage(websocket.BinaryMessage, []byte{byte(MsgReady)})
		if err != nil {
			return
		}

		w, err := conn.NextWriter(t)
		if err != nil {
			return
		}

		_, err = io.Copy(w, r)
		if err != nil {
			return
		}
		w.Close()
	}
}

func startServerEcho(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", serveEcho)
	//http.HandleFunc("/", serveEcho)
	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	}
	go httpSrv.ListenAndServe()
	return httpSrv
}

var conn *websocket.Conn

func TestMain(m *testing.M) {
	var err error

	addr := getLocalAddress()
	httpSrv := startServerEcho(addr)
	defer httpSrv.Close()

	u := url.URL{Scheme: "ws", Host: addr, Path: "/"}
	conn, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		os.Stderr.WriteString("cannot start websocket server: " + err.Error())
		os.Exit(1)
	}
	defer conn.Close()

	os.Exit(m.Run())
}

func TestSendJSONData(t *testing.T) {
	sampleData := map[string]string{
		"key1": "val1",
		"key2": "val2",
		"key3": "val3",
	}

	err := SendJSONData(conn, MsgSmbios, sampleData)
	assert.NilError(t, err)

	_, rData, err := conn.ReadMessage()
	assert.NilError(t, err)

	rType, rData, err := extractMessageType(rData)
	assert.NilError(t, err)
	assert.Equal(t, MsgSmbios, rType)

	checkData := map[string]string{}
	err = json.Unmarshal(rData, &checkData)
	assert.NilError(t, err)
	for k, v := range sampleData {
		val := checkData[k]
		assert.Equal(t, val, v)
	}
}

func TestReadWriteMessage(t *testing.T) {
	testCase := []struct {
		mType MessageType
		mData []byte
	}{
		{
			mType: MsgReady,
			mData: []byte("abcdefghilmnopqrstuvz123345"),
		},
		{
			mType: MsgSmbios,
			mData: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40,
				41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60,
				61, 62, 63, 64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80,
				81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 91, 92, 93, 94, 95, 96, 97, 98, 99, 100,
				101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116,
				117, 118, 119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 129, 130, 131, 132,
				133, 134, 135, 136, 137, 138, 139, 140, 141, 142, 143, 144, 145, 146, 147, 148,
				149, 150, 151, 152, 153, 154, 155, 156, 157, 158, 159, 160, 161, 162, 163, 164,
				165, 166, 167, 168, 169, 170, 171, 172, 173, 174, 175, 176, 177, 178, 179, 180,
				181, 182, 183, 184, 185, 186, 187, 188, 189, 190, 191, 192, 193, 194, 195, 196,
				197, 198, 199, 200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212,
				213, 214, 215, 216, 217, 218, 219, 220, 221, 222, 223, 224, 225, 226, 227, 228,
				229, 230, 231, 232, 233, 234, 235, 236, 237, 238, 239, 240, 241, 242, 243, 244,
				245, 246, 247, 248, 249, 250, 251, 252, 253, 254, 255},
		},
		{
			mType: MsgLabels,
			// exceed default buffer size of 4K: 50 * 100 bytes byte array
			mData: []byte(`
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
abcdefghilmnopqrstuvz1233456789WQERTYUIOPASDFGHJKLZXCVBNMabcdefghilmnopqrstuvz1233456789WQERTYUIOPAS
				`),
		},
	}

	for _, test := range testCase {
		err := WriteMessage(conn, test.mType, test.mData)
		if err != nil {
			t.Errorf("WriteMessage: %s", err.Error())
		}

		// Expecting ack from the server
		rType, _, err := ReadMessage(conn)
		assert.NilError(t, err)
		assert.Equal(t, rType, MsgReady)

		rType, rMsg, err := ReadMessage(conn)
		if err != nil {
			t.Errorf("ReadMessage: %s", err.Error())
		}

		assert.Equal(t, rType, test.mType)
		assert.Equal(t, string(rMsg), string(test.mData))
	}
}
