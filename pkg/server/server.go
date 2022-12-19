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
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/register"
	"github.com/rancher/elemental-operator/pkg/tpm"
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
			tpm.New(ctx, cl),
		},
	}

	return server
}

func (i *InventoryServer) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	escapedPath := strings.Replace(req.URL.Path, "\n", "", -1)
	escapedPath = strings.Replace(escapedPath, "\r", "", -1)
	logrus.Infof("Incoming HTTP request for %s", escapedPath)

	// expect /elemental/{api}...
	splittedPath := strings.Split(escapedPath, "/")
	if len(splittedPath) < 3 {
		http.Error(resp, "", http.StatusNotFound)
		return
	}
	// drop 'elemental'
	splittedPath = splittedPath[2:]
	api := splittedPath[0]

	switch api {
	case "build-image":
		if err := i.apiBuildImage(resp, req); err != nil {
			logrus.Errorf("build-image: %s", err.Error())
			return
		}
	case "registration":
		if err := i.apiRegistration(resp, req); err != nil {
			logrus.Errorf("registration: %s", err.Error())
			return
		}
	default:
		logrus.Errorf("Unknown API: %s", api)
		http.Error(resp, fmt.Sprintf("unknwon api: %s", api), http.StatusBadRequest)
		return
	}
}

func upgrade(resp http.ResponseWriter, req *http.Request) (*websocket.Conn, error) {
	upgrader := websocket.Upgrader{
		HandshakeTimeout: 5 * time.Second,
		CheckOrigin:      func(r *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(resp, req, nil)
	if err != nil {
		return nil, err
	}
	_ = conn.SetWriteDeadline(time.Now().Add(register.RegistrationDeadlineSeconds * time.Second))
	_ = conn.SetReadDeadline(time.Now().Add(register.RegistrationDeadlineSeconds * time.Second))

	return conn, err
}

func (i *InventoryServer) getValue(name string) (string, error) {
	setting := &managementv3.Setting{}
	if err := i.Get(i, types.NamespacedName{Name: name}, setting); err != nil {
		logrus.Errorf("Error getting %s setting: %s", name, err.Error())
		return "", err
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

func initInventory(inventory *elementalv1.MachineInventory, registration *elementalv1.MachineRegistration) {
	const namePrefix = "m-"

	if registration.Spec.Config == nil {
		registration.Spec.Config = &elementalv1.Config{}
	}
	inventory.Name = registration.Spec.MachineName
	if inventory.Name == "" {
		if registration.Spec.Config.Elemental.Registration.NoSMBIOS {
			inventory.Name = namePrefix + uuid.NewString()
		} else {
			inventory.Name = namePrefix + "${System Information/UUID}"
		}
	}
	inventory.Namespace = registration.Namespace
	inventory.Annotations = registration.Spec.MachineInventoryAnnotations
	inventory.Labels = registration.Spec.MachineInventoryLabels
}

func (i *InventoryServer) createMachineInventory(inventory *elementalv1.MachineInventory) (*elementalv1.MachineInventory, error) {

	if inventory.Spec.TPMHash == "" {
		return nil, fmt.Errorf("machine inventory TPMHash is empty")
	}

	mInventoryList := &elementalv1.MachineInventoryList{}

	if err := i.List(i, mInventoryList); err != nil {
		return nil, fmt.Errorf("cannot retrieve machine inventory list: %w", err)
	}

	// TODO: add a cache map for machine inventories indexed by TPMHash
	for _, m := range mInventoryList.Items {
		if m.Spec.TPMHash == inventory.Spec.TPMHash {
			return nil, fmt.Errorf("machine inventory with TPM hash %s already present: %s/%s",
				m.Spec.TPMHash, m.Namespace, m.Name)
		}
	}

	if err := i.Create(i, inventory); err != nil {
		return nil, fmt.Errorf("failed to create machine inventory %s/%s: %w", inventory.Namespace, inventory.Name, err)
	}

	logrus.Infof("new machine inventory created: %s", inventory.Name)

	return inventory, nil
}

func (i *InventoryServer) updateMachineInventory(inventory *elementalv1.MachineInventory) (*elementalv1.MachineInventory, error) {
	if err := i.Update(i, inventory); err != nil {
		return nil, fmt.Errorf("failed to update machine inventory %s/%s: %w", inventory.Namespace, inventory.Name, err)
	}

	logrus.Infof("machine inventory updated: %s", inventory.Name)

	return inventory, nil
}

func (i *InventoryServer) commitMachineInventory(inventory *elementalv1.MachineInventory) (*elementalv1.MachineInventory, error) {
	var err error
	if inventory.CreationTimestamp.IsZero() {
		if inventory, err = i.createMachineInventory(inventory); err != nil {
			return nil, fmt.Errorf("MachineInventory creation failed: %w", err)
		}
	} else {
		if inventory, err = i.updateMachineInventory(inventory); err != nil {
			return nil, fmt.Errorf("MachineInventory update failed: %w", err)
		}
	}
	return inventory, nil
}

func (i *InventoryServer) getMachineRegistration(token string) (*elementalv1.MachineRegistration, error) {
	escapedToken := strings.Replace(token, "\n", "", -1)
	escapedToken = strings.Replace(escapedToken, "\r", "", -1)

	mRegistrationList := &elementalv1.MachineRegistrationList{}
	if err := i.List(i, mRegistrationList); err != nil {
		return nil, fmt.Errorf("failed to list machine registrations")
	}

	var mRegistration *elementalv1.MachineRegistration

	// TODO: build machine registrations cache indexed by token
	for _, m := range mRegistrationList.Items {
		if m.Status.RegistrationToken == escapedToken {
			// Found two registrations with the same registration token
			if mRegistration != nil {
				return nil, fmt.Errorf("machine registrations %s/%s and %s/%s have the same registration token %s",
					mRegistration.Namespace, mRegistration.Name, m.Namespace, m.Name, escapedToken)
			}
			mRegistration = (&m).DeepCopy()
		}
	}

	if mRegistration == nil {
		return nil, fmt.Errorf("failed to find machine registration with registration token %s", escapedToken)
	}

	var ready bool
	for _, condition := range mRegistration.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == "True" {
			ready = true
			break
		}
	}

	if !ready {
		return nil, fmt.Errorf("MachineRegistration %s/%s is not ready", mRegistration.Namespace, mRegistration.Name)
	}

	return mRegistration, nil
}
