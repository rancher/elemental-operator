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

package plainauth

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/gorilla/websocket"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/sirupsen/logrus"
)

type AuthClient struct {
	macAddress []byte
}

func (auth *AuthClient) Init(reg elementalv1.Registration) error {
	if auth.macAddress != nil {
		logrus.Debugf("MAC Plan Authentication was already initialized: %x", auth.macAddress)
		return nil
	}

	mac, err := GetHostMacAddr()
	if err != nil {
		return err
	}
	logrus.Debugf("Plain authentication MAC:%x", mac)
	auth.macAddress = mac
	return nil
}

func (auth *AuthClient) GetName() string {
	return "Plain (MAC-based)"
}

func (auth *AuthClient) GetToken() (string, error) {
	if auth.macAddress == nil {
		return "", fmt.Errorf("plainauth data is not initialized: have you called Init()?")
	}
	return "Bearer PLAIN" + base64.StdEncoding.EncodeToString(auth.macAddress), nil
}

func (auth *AuthClient) GetPubHash() (string, error) {
	if auth.macAddress == nil {
		return "", fmt.Errorf("plainauth data is not initialized: have you called Init()?")
	}
	pubHash := sha256.Sum256(auth.macAddress)
	hashEncoded := fmt.Sprintf("%x", pubHash)
	return hashEncoded, nil
}

func (auth *AuthClient) Authenticate(conn *websocket.Conn) error {
	return nil
}
