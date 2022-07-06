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
	"io"
	"net/http"

	elm "github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/clients"
	elmcontrollers "github.com/rancher/elemental-operator/pkg/generated/controllers/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/tpm"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
)

var (
	registrationTokenIndex = "registrationTokenIndex"
	tpmHashIndex           = "tpmHashIndex"
)

type authenticator interface {
	Authenticate(resp http.ResponseWriter, req *http.Request, registerNamespace string) (*elm.MachineInventory, bool, io.WriteCloser, error)
}

type InventoryServer struct {
	secretCache              corecontrollers.SecretCache
	serviceAccountCache      corecontrollers.ServiceAccountCache
	machineCache             elmcontrollers.MachineInventoryCache
	machineClient            elmcontrollers.MachineInventoryClient
	machineRegistrationCache elmcontrollers.MachineRegistrationCache
	authenticators           []authenticator
	serverURL                string
	caCerts                  string
}

func New(clients *clients.Clients, serverURL, caCerts string) *InventoryServer {
	server := &InventoryServer{
		authenticators: []authenticator{
			tpm.New(clients),
		},
		secretCache:              clients.Core.Secret().Cache(),
		serviceAccountCache:      clients.Core.ServiceAccount().Cache(),
		machineCache:             clients.Elemental.MachineInventory().Cache(),
		machineClient:            clients.Elemental.MachineInventory(),
		machineRegistrationCache: clients.Elemental.MachineRegistration().Cache(),
		serverURL:                serverURL,
		caCerts:                  caCerts,
	}

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

func (i *InventoryServer) authMachine(resp http.ResponseWriter, req *http.Request, registerNamespace string) (*elm.MachineInventory, io.WriteCloser, error) {
	for _, auth := range i.authenticators {
		machine, cont, writer, err := auth.Authenticate(resp, req, registerNamespace)
		if err != nil {
			return nil, nil, err
		}
		if machine != nil || !cont {
			return machine, writer, nil
		}
	}
	return nil, nil, nil
}
