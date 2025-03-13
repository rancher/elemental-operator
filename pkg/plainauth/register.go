/*
Copyright © 2022 - 2025 SUSE LLC

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
	"fmt"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/jaypipes/ghw"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/log"
)

type AuthClient struct {
	idType string
	idVal  string
}

func isNotFilled(value string) bool {
	if value == "" || value == "Unspecified" || value == "None" {
		return true
	}
	// "Not Present", "Not Specified", "Not [...]"
	if strings.HasPrefix(value, "Not ") {
		return true
	}
	return false
}

func (auth *AuthClient) Init(reg elementalv1.Registration) error {
	if auth.idVal != "" {
		log.Debugf("Plain Authentication with id type %s is already initialized: %s", auth.idType, auth.idVal)
		return nil
	}

	switch reg.Auth {
	case "sys-uuid":
		product, err := ghw.Product(ghw.WithDisableWarnings())
		if err != nil {
			return fmt.Errorf("cannot access SMBIOS data: %w", err)
		}
		uuid := product.UUID
		if isNotFilled(uuid) {
			return fmt.Errorf("SMBIOS UUID is empty")
		}
		auth.idType = reg.Auth
		auth.idVal = uuid
	case "mac":
		mac, err := getHostMacAddr()
		if err != nil {
			return fmt.Errorf("cannot get MAC address: %w", err)
		}
		auth.idType = reg.Auth
		auth.idVal = mac
	default:
		return fmt.Errorf("unknown authentication type: %s", auth.idType)
	}

	log.Infof("Plain authentication: %s %s", auth.idType, auth.idVal)
	return nil
}

func (auth *AuthClient) GetName() string {
	return "Plain"
}

func (auth *AuthClient) GetToken() (string, error) {
	if auth.idVal == "" {
		return "", fmt.Errorf("plainauth data is not initialized: have you called Init()?")
	}
	return "Bearer PLAIN" + auth.idVal, nil
}

func (auth *AuthClient) GetPubHash() (string, error) {
	if auth.idVal == "" {
		return "", fmt.Errorf("plainauth data is not initialized: have you called Init()?")
	}
	pubHash := sha256.Sum256([]byte(auth.idVal))
	hashEncoded := fmt.Sprintf("%x", pubHash)
	return hashEncoded, nil
}

func (auth *AuthClient) Authenticate(_ *websocket.Conn) error {
	return nil
}

func getHostMacAddr() (string, error) {
	netInfo, err := ghw.Network(ghw.WithDisableWarnings())
	if err != nil {
		return "", fmt.Errorf("cannot access networking data: %w", err)
	}

	hwAddr := ""
	for _, nic := range netInfo.NICs {
		if nic.IsVirtual || nic.MacAddress == "" || nic.MacAddress == "00:00:00:00:00:00" {
			continue
		}
		if hwAddr == "" {
			hwAddr = nic.MacAddress
			continue
		}

		// pick the "lower" MAC address
		if strings.Compare(hwAddr, nic.MacAddress) > 0 {
			hwAddr = nic.MacAddress
			continue
		}
	}

	if hwAddr == "" {
		return "", fmt.Errorf("cannot retrieve MAC address from any network interface")
	}

	return hwAddr, nil
}
