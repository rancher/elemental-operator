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

package server

import (
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	elm "github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/clients"
	elmcontrollers "github.com/rancher/elemental-operator/pkg/generated/controllers/elemental.cattle.io/v1beta1"
	ranchercontrollers "github.com/rancher/elemental-operator/pkg/generated/controllers/management.cattle.io/v3"
	"github.com/rancher/elemental-operator/pkg/tpm"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
)

var (
	registrationTokenIndex = "registrationTokenIndex"
	tpmHashIndex           = "tpmHashIndex"
	settingsIndex          = "settingsIndex"
)

type authenticator interface {
	Authenticate(conn *websocket.Conn, req *http.Request, registerNamespace string) (*elm.MachineInventory, bool, error)
}

type InventoryServer struct {
	secretCache              corecontrollers.SecretCache
	serviceAccountCache      corecontrollers.ServiceAccountCache
	machineCache             elmcontrollers.MachineInventoryCache
	machineClient            elmcontrollers.MachineInventoryClient
	machineRegistrationCache elmcontrollers.MachineRegistrationCache
	settingCache             ranchercontrollers.SettingCache
	authenticators           []authenticator
}

func New(clients *clients.Clients) *InventoryServer {
	server := &InventoryServer{
		authenticators: []authenticator{
			tpm.New(clients),
		},
		secretCache:              clients.Core().Secret().Cache(),
		serviceAccountCache:      clients.Core().ServiceAccount().Cache(),
		machineCache:             clients.Elemental().MachineInventory().Cache(),
		machineClient:            clients.Elemental().MachineInventory(),
		machineRegistrationCache: clients.Elemental().MachineRegistration().Cache(),
		settingCache:             clients.Rancher().Setting().Cache(),
	}

	server.settingCache.AddIndexer(settingsIndex, func(obj *v3.Setting) ([]string, error) {
		if obj.Value == "" {
			return nil, nil
		}
		return []string{obj.Value}, nil
	})

	server.machineRegistrationCache.AddIndexer(registrationTokenIndex, func(obj *elm.MachineRegistration) ([]string, error) {
		if obj.Status.RegistrationToken == "" {
			return nil, nil
		}
		return []string{
			obj.Status.RegistrationToken,
		}, nil
	})

	server.machineCache.AddIndexer(tpmHashIndex, func(obj *elm.MachineInventory) ([]string, error) {
		if obj.Spec.TPMHash == "" {
			return nil, nil
		}
		return []string{obj.Spec.TPMHash}, nil
	})

	return server
}

func (i *InventoryServer) getRancherCACert() string {
	setting, err := i.settingCache.Get("cacerts")
	if err != nil {
		logrus.Errorf("Error getting cacerts setting: %s", err.Error())
		return ""
	}
	if setting.Value == "" {
		setting, err = i.settingCache.Get("internal-cacerts")
		if err != nil {
			logrus.Errorf("Error getting internal-cacerts setting: %s", err.Error())
			return ""
		}
	}
	return setting.Value
}

func (i *InventoryServer) getRancherServerURL() (string, error) {
	setting, err := i.settingCache.Get("server-url")
	if err != nil {
		return "", err
	}
	if setting.Value == "" {
		return "", fmt.Errorf("server-url is not set")
	}
	return setting.Value, nil
}

func (i *InventoryServer) authMachine(conn *websocket.Conn, req *http.Request, registerNamespace string) (*elm.MachineInventory, error) {
	for _, auth := range i.authenticators {
		machine, cont, err := auth.Authenticate(conn, req, registerNamespace)
		if err != nil {
			return nil, err
		}
		if machine != nil || !cont {
			return machine, nil
		}
	}
	return nil, nil
}
