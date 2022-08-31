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

package tpm

import (
	"github.com/gorilla/websocket"
	gotpm "github.com/rancher-sandbox/go-tpm"
)

type AttestationChannel struct {
	conn *websocket.Conn
}

func (att *AttestationChannel) Read(p []byte) (int, error) {
	_, r, err := att.conn.NextReader()
	if err != nil {
		return 0, err
	}
	return r.Read(p)
}

func (att *AttestationChannel) Write(p []byte) (int, error) {
	w, err := att.conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return 0, err
	}
	defer w.Close()

	return w.Write(p)
}

type AuthClient struct {
	emulateTPM bool
	seed       int64
	ak         []byte
}

func (auth *AuthClient) EmulateTPM(seed int64) {
	auth.emulateTPM = true
	auth.seed = seed
}

func (auth *AuthClient) GetAuthToken() (string, string, error) {
	var opts []gotpm.Option
	if auth.emulateTPM {
		opts = append(opts, gotpm.Emulated)
		opts = append(opts, gotpm.WithSeed(auth.seed))
	}
	token, akBytes, err := gotpm.GetAuthToken(opts...)
	if err != nil {
		return "", "", err
	}
	auth.ak = akBytes

	hash, err := gotpm.GetPubHash(opts...)
	if err != nil {
		return "", "", err
	}
	return token, hash, nil
}

func (auth *AuthClient) Init(conn *websocket.Conn) error {
	var opts []gotpm.Option
	if auth.emulateTPM {
		opts = append(opts, gotpm.Emulated)
		opts = append(opts, gotpm.WithSeed(auth.seed))
	}

	return gotpm.Authenticate(auth.ak, &AttestationChannel{conn}, opts...)
}
