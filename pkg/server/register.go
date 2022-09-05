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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	elm "github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/config"
	"github.com/rancher/elemental-operator/pkg/register"
	values "github.com/rancher/wrangler/pkg/data"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	sanitize         = regexp.MustCompile("[^0-9a-zA-Z_]")
	sanitizeHostname = regexp.MustCompile("[^0-9a-zA-Z]")
	doubleDash       = regexp.MustCompile("--+")
	start            = regexp.MustCompile("^[a-zA-Z0-9]")
)

func (i *InventoryServer) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	logrus.Infof("Incoming HTTP request for %s", req.URL.Path)
	// get the machine registration relevant to this request
	registration, err := i.getMachineRegistration(req)
	if err != nil {
		http.Error(resp, err.Error(), http.StatusNotFound)
		return
	}

	if !websocket.IsWebSocketUpgrade(req) {
		logrus.Debug("got a plain HTTP request: send unauthenticated registration")
		if err = i.unauthenticatedResponse(registration, resp); err != nil {
			logrus.Error("error sending unauthenticated response: ", err)
		}
		return
	}

	// upgrade to websocket
	conn, err := upgrade(resp, req)
	if err != nil {
		logrus.Error("failed to upgrade connection to websocket: %w", err)
		return
	}
	defer conn.Close()
	logrus.Debug("connection upgraded to websocket")

	// attempt to authenticate the machine, if err, authentication has failed
	inventory, err := i.authMachine(conn, req, registration.Namespace)
	if err != nil {
		logrus.Error("authentication failed: ", err)
		return
	}
	// no error and no inventory: Auth header is missing or unrecognized
	if inventory == nil {
		logrus.Warn("websocket connection: unrecognized authentication method")
		if err = i.unauthenticatedResponse(registration, resp); err != nil {
			logrus.Error("error sending unauthenticated response: ", err)
		}
		return
	}
	logrus.Debug("attestation completed")

	if err := register.WriteMessage(conn, register.MsgReady, []byte{}); err != nil {
		logrus.Error("cannot finalize the authentication process: %w", err)
		return
	}

	isNewInventory := inventory.CreationTimestamp.IsZero()
	if isNewInventory {
		initInventory(inventory, registration)
	}

	_, err = i.serveLoop(conn, inventory, registration)
	if err != nil {
		logrus.Error(err)
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

func (i *InventoryServer) unauthenticatedResponse(machineRegistration *elm.MachineRegistration, writer io.Writer) error {
	mRRegistration := machineRegistration.Spec.Config.Elemental.Registration

	return yaml.NewEncoder(writer).Encode(config.Config{
		Elemental: config.Elemental{
			Registration: config.Registration{
				URL:             machineRegistration.Status.RegistrationURL,
				CACert:          i.getRancherCACert(),
				EmulateTPM:      mRRegistration.EmulateTPM,
				EmulatedTPMSeed: mRRegistration.EmulatedTPMSeed,
				NoSMBIOS:        mRRegistration.NoSMBIOS,
			},
		},
	})
}

func (i *InventoryServer) createMachineInventory(inventory *elm.MachineInventory) (*elm.MachineInventory, error) {
	machines, err := i.machineCache.GetByIndex(tpmHashIndex, inventory.Spec.TPMHash)
	if err != nil || len(machines) > 0 {
		return nil, err
	}
	if len(machines) > 0 {
		return nil, fmt.Errorf("machine with TPM hash %s is already registered", machines[0].Spec.TPMHash)
	}

	inventory, err = i.machineClient.Create(inventory)
	if err == nil {
		logrus.Infof("new machine inventory created: %s", inventory.Name)
	}
	return inventory, err
}

func (i *InventoryServer) updateMachineInventory(inventory *elm.MachineInventory) (*elm.MachineInventory, error) {
	inventory, err := i.machineClient.Update(inventory)
	if err == nil {
		logrus.Infof("machine inventory updated: %s", inventory.Name)
	}
	return inventory, err
}

func (i *InventoryServer) getMachineRegistration(req *http.Request) (*elm.MachineRegistration, error) {
	token := path.Base(req.URL.Path)

	regs, err := i.machineRegistrationCache.GetByIndex(registrationTokenIndex, token)
	if apierrors.IsNotFound(err) || len(regs) != 1 {
		if len(regs) > 1 {
			logrus.Errorf("Multiple MachineRegistrations have the same token %s: %v", token, regs)
		}
		if err == nil && len(regs) == 0 {
			err = fmt.Errorf("MachineRegistration does not exist")
		}
		return nil, err
	}

	var ready bool
	for _, condition := range regs[0].Status.Conditions {
		if condition.Type == "Ready" && condition.Status == "True" {
			ready = true
			break
		}
	}

	if !ready {
		return nil, errors.New("MachineRegistration is not ready")
	}

	return regs[0], nil

}

func (i *InventoryServer) writeMachineInventoryCloudConfig(conn *websocket.Conn, inventory *elm.MachineInventory, registration *elm.MachineRegistration) error {
	var err error

	sa, err := i.serviceAccountCache.Get(registration.Status.ServiceAccountRef.Namespace,
		registration.Status.ServiceAccountRef.Name)
	if err != nil || len(sa.Secrets) < 1 {
		return err
	}

	tokenSecret, err := i.secretCache.Get(sa.Namespace, sa.Secrets[0].Name)
	if err != nil || tokenSecret.Type != v1.SecretTypeServiceAccountToken {
		return err
	}

	serverURL, err := i.getRancherServerURL()
	if err != nil {
		return fmt.Errorf("failed to get server-url: %s", err.Error())
	}

	writer, err := conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return err
	}
	defer writer.Close()

	return yaml.NewEncoder(writer).Encode(config.Config{
		Elemental: config.Elemental{
			Registration: config.Registration{
				URL:    registration.Status.RegistrationURL,
				CACert: i.getRancherCACert(),
			},
			SystemAgent: config.SystemAgent{
				URL:             fmt.Sprintf("%s/k8s/clusters/local", serverURL),
				Token:           string(tokenSecret.Data["token"]),
				SecretName:      inventory.Name,
				SecretNamespace: inventory.Namespace,
			},
			Install: registration.Spec.Config.Elemental.Install,
		},
		CloudConfig: registration.Spec.Config.CloudConfig,
	})
}

func buildStringFromSmbiosData(data map[string]interface{}, name string) string {
	str := name
	result := &strings.Builder{}
	for {
		i := strings.Index(str, "${")
		if i == -1 {
			result.WriteString(str)
			break
		}
		j := strings.Index(str[i:], "}")
		if j == -1 {
			result.WriteString(str)
			break
		}

		result.WriteString(str[:i])
		obj := values.GetValueN(data, strings.Split(str[i+2:j+i], "/")...)
		if str, ok := obj.(string); ok {
			result.WriteString(str)
		}
		str = str[j+i+1:]
	}

	resultStr := sanitize.ReplaceAllString(result.String(), "-")
	resultStr = doubleDash.ReplaceAllString(resultStr, "-")
	if !start.MatchString(resultStr) {
		resultStr = "m" + resultStr
	}
	if len(resultStr) > 58 {
		resultStr = resultStr[:58]
	}
	return resultStr
}

func initInventory(inventory *elm.MachineInventory, registration *elm.MachineRegistration) {
	const namePrefix = "m-"

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

func (i *InventoryServer) serveLoop(conn *websocket.Conn, inventory *elm.MachineInventory, reg *elm.MachineRegistration) (msgType register.MessageType, err error) {
	updated := false
	for {
		var data []byte

		msgType, data, err = register.ReadMessage(conn)
		if err != nil {
			return msgType, fmt.Errorf("websocket communication interrupted: %w", err)
		}

		switch msgType {
		case register.MsgSmbios:
			smbiosData := map[string]interface{}{}
			if err = json.Unmarshal(data, &smbiosData); err != nil {
				return msgType, fmt.Errorf("cannot extract SMBios data: %w", err)
			}
			// Sanitize any lower dashes into dashes as hostnames cannot have lower dashes, and we use the inventory name
			// to set the machine hostname. Also set it to lowercase
			inventory.Name = strings.ToLower(sanitizeHostname.ReplaceAllString(buildStringFromSmbiosData(smbiosData, inventory.Name), "-"))
			logrus.Debug("Adding labels from registration")
			// Add extra label info from data coming from smbios and based on the registration data
			if inventory.Labels == nil {
				inventory.Labels = map[string]string{}
			}
			for k, v := range reg.Spec.MachineInventoryLabels {
				parsedData := buildStringFromSmbiosData(smbiosData, v)
				logrus.Debugf("Parsed %s into %s with smbios data, setting it to label %s", v, parsedData, k)
				inventory.Labels[k] = strings.TrimSuffix(strings.TrimPrefix(parsedData, "-"), "-")
			}
			logrus.Debugf("received SMBIOS data - generated machine name: %s", inventory.Name)
		case register.MsgLabels:
			if err := mergeInventoryLabels(inventory, data); err != nil {
				return msgType, err
			}
		case register.MsgGet:
			inventory, err = i.commitMachineInventory(inventory, updated)
			if err != nil {
				return msgType, err
			}
			err = i.writeMachineInventoryCloudConfig(conn, inventory, reg)
			if err != nil {
				return msgType, fmt.Errorf("failed sending elemental cloud config: %w", err)
			}
			logrus.Debug("elemental cloud config sent")
			return msgType, nil

		default:
			return msgType, fmt.Errorf("got unexpected message: %s", msgType)
		}
		if err := register.WriteMessage(conn, register.MsgReady, []byte{}); err != nil {
			return msgType, fmt.Errorf("cannot complete %s exchange", msgType)
		}
	}
}

func mergeInventoryLabels(inventory *elm.MachineInventory, data []byte) error {
	labels := map[string]string{}
	if err := json.Unmarshal(data, &labels); err != nil {
		return fmt.Errorf("cannot extract inventory labels: %w", err)
	}
	logrus.Debugf("received labels: %v", labels)
	logrus.Warn("received labels from registering client: no more supported, skipping")
	if inventory.Labels == nil {
		inventory.Labels = map[string]string{}
	}
	return nil
}

func (i *InventoryServer) commitMachineInventory(inventory *elm.MachineInventory, updated bool) (*elm.MachineInventory, error) {
	var err error
	if inventory.CreationTimestamp.IsZero() {
		if inventory, err = i.createMachineInventory(inventory); err != nil {
			return nil, fmt.Errorf("MachineInventory creation failed: %w", err)
		}
	} else if updated {
		if inventory, err = i.updateMachineInventory(inventory); err != nil {
			return nil, fmt.Errorf("MachineInventory update failed: %w", err)
		}
	}
	return inventory, nil
}
