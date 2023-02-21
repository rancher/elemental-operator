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

package tpm

import (
	"math/big"
	"math/rand"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/jaypipes/ghw"
	gotpm "github.com/rancher-sandbox/go-tpm"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/log"
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

func (auth *AuthClient) Init(reg elementalv1.Registration) error {
	if reg.EmulateTPM {
		emulatedSeed := reg.EmulatedTPMSeed
		log.Infof("Enable TPM emulation")
		if emulatedSeed == -1 {
			data, err := ghw.Product(ghw.WithDisableWarnings())
			if err != nil {
				emulatedSeed = rand.Int63()
				log.Debugf("TPM emulation using random seed: %d", emulatedSeed)
			} else {
				uuid := strings.Replace(data.UUID, "-", "", -1)
				var i big.Int
				_, converted := i.SetString(uuid, 16)
				if !converted {
					emulatedSeed = rand.Int63()
					log.Debugf("TPM emulation using random seed: %d", emulatedSeed)
				} else {
					emulatedSeed = i.Int64()
					log.Debugf("TPM emulation using system UUID %s, resulting in seed: %d", uuid, emulatedSeed)
				}
			}
		}
		auth.emulateTPM = true
		auth.seed = emulatedSeed
	}
	return nil
}

func (auth *AuthClient) Authenticate(conn *websocket.Conn) error {
	var opts []gotpm.Option
	if auth.emulateTPM {
		opts = append(opts, gotpm.Emulated)
		opts = append(opts, gotpm.WithSeed(auth.seed))
	}

	return gotpm.Authenticate(auth.ak, &AttestationChannel{conn}, opts...)
}

func (auth *AuthClient) GetName() string {
	return "TPM"
}

func (auth *AuthClient) GetToken() (string, error) {
	var opts []gotpm.Option
	if auth.emulateTPM {
		opts = append(opts, gotpm.Emulated)
		opts = append(opts, gotpm.WithSeed(auth.seed))
	}
	token, akBytes, err := gotpm.GetAuthToken(opts...)
	if err != nil {
		return "", err
	}
	auth.ak = akBytes

	return token, nil
}

func (auth *AuthClient) GetPubHash() (string, error) {
	var opts []gotpm.Option
	if auth.emulateTPM {
		opts = append(opts, gotpm.Emulated)
		opts = append(opts, gotpm.WithSeed(auth.seed))
	}
	return gotpm.GetPubHash(opts...)
}
