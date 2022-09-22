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
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/tpm"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type authenticator interface {
	Authenticate(conn *websocket.Conn, req *http.Request, registerNamespace string) (*elementalv1.MachineInventory, bool, error)
}

type InventoryServer struct {
	client.Client
	context.Context
	authenticators []authenticator
}

func New(ctx context.Context, cl client.Client) *InventoryServer {
	server := &InventoryServer{
		Client:  cl,
		Context: ctx,
		authenticators: []authenticator{
			tpm.New(cl),
		},
	}
	return server
}

func (i *InventoryServer) getRancherCACert() string {
	setting := &managementv3.Setting{}
	if err := i.Get(i, types.NamespacedName{Name: "cacerts"}, setting); err != nil {
		logrus.Errorf("Error getting cacerts setting: %s", err.Error())
		return ""
	}

	if setting.Value == "" {
		if err := i.Get(i, types.NamespacedName{Name: "internal-cacerts"}, setting); err != nil {
			logrus.Errorf("Error getting internal-cacerts setting: %s", err.Error())
			return ""
		}
	}
	return setting.Value
}

func (i *InventoryServer) getRancherServerURL() (string, error) {
	setting := &managementv3.Setting{}
	if err := i.Get(i, types.NamespacedName{Name: "server-url"}, setting); err != nil {
		return "", fmt.Errorf("failed to get server url setting: %w", err)
	}

	if setting.Value == "" {
		return "", errors.New("server-url is not set")
	}

	return setting.Value, nil
}

func (i *InventoryServer) authMachine(conn *websocket.Conn, req *http.Request, registerNamespace string) (*elementalv1.MachineInventory, error) {
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
