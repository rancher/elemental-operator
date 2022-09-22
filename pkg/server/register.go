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
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/config"
	"github.com/rancher/elemental-operator/pkg/register"
	values "github.com/rancher/wrangler/pkg/data"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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

func (i *InventoryServer) unauthenticatedResponse(machineRegistration *elementalv1.MachineRegistration, writer io.Writer) error {
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

func (i *InventoryServer) createMachineInventory(inventory *elementalv1.MachineInventory) (*elementalv1.MachineInventory, error) {
	mInventoryList := elementalv1.MachineInventoryList{}

	if err := i.List(i, &mInventoryList); err != nil {
		return nil, err
	}

	for _, mi := range mInventoryList.Items {
		if inventory.Spec.TPMHash != "" && mi.Spec.TPMHash == inventory.Spec.TPMHash {
			return nil, fmt.Errorf("machine inventories with TPM hash %s is already registered", inventory.Spec.TPMHash)
		}
	}

	if err := i.Create(i, inventory); err != nil {
		return nil, fmt.Errorf("failed to create new machine inventory: %w", err)
	}

	return inventory, nil
}

func (i *InventoryServer) updateMachineInventory(inventory *elementalv1.MachineInventory) (*elementalv1.MachineInventory, error) {
	if err := i.Update(i, inventory); err != nil {
		return nil, fmt.Errorf("failed to update new machine inventory: %w", err)
	}
	return inventory, nil
}

func (i *InventoryServer) getMachineRegistration(req *http.Request) (*elementalv1.MachineRegistration, error) {
	token := path.Base(req.URL.Path)

	mRegistrationList := &elementalv1.MachineRegistrationList{}

	if err := i.List(i, mRegistrationList); err != nil {
		return nil, fmt.Errorf("failed to list machine registrations")
	}

	tokenMap := map[string]string{}
	mRegistration := &elementalv1.MachineRegistration{}
	for _, mr := range mRegistrationList.Items {
		if mr.Status.RegistrationToken == token {
			if _, ok := tokenMap[token]; ok {
				return nil, fmt.Errorf("registration with this token already exists")
			}
			mRegistration = &mr
			tokenMap[mr.Status.RegistrationToken] = ""
		}

	}

	var ready bool
	for _, condition := range mRegistration.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == "True" {
			ready = true
			break
		}
	}

	if !ready {
		return nil, errors.New("MachineRegistration is not ready")
	}

	return mRegistration, nil

}

func (i *InventoryServer) writeMachineInventoryCloudConfig(conn *websocket.Conn, inventory *elementalv1.MachineInventory, registration *elementalv1.MachineRegistration) error {
	var err error

	sa := &corev1.ServiceAccount{}
	if err := i.Get(i, types.NamespacedName{
		Name:      registration.Status.ServiceAccountRef.Name,
		Namespace: registration.Status.ServiceAccountRef.Namespace,
	}, sa); err != nil {
		return fmt.Errorf("failed to get service account: %v", err)
	}

	tokenSecret := &corev1.Secret{}
	if err := i.Get(i, types.NamespacedName{
		Name:      sa.Secrets[0].Name,
		Namespace: sa.Namespace,
	}, tokenSecret); err != nil || tokenSecret.Type != corev1.SecretTypeServiceAccountToken {
		return fmt.Errorf("failed to get secret: %v", err)
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

	return yaml.NewEncoder(writer).Encode(elementalv1.Config{
		Elemental: elementalv1.Elemental{
			Registration: elementalv1.Registration{
				URL:    registration.Status.RegistrationURL,
				CACert: i.getRancherCACert(),
			},
			SystemAgent: elementalv1.SystemAgent{
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

func initInventory(inventory *elementalv1.MachineInventory, registration *elementalv1.MachineRegistration) {
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

func (i *InventoryServer) serveLoop(conn *websocket.Conn, inventory *elementalv1.MachineInventory, reg *elementalv1.MachineRegistration) (msgType register.MessageType, err error) {
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

func mergeInventoryLabels(inventory *elementalv1.MachineInventory, data []byte) error {
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

func (i *InventoryServer) commitMachineInventory(inventory *elementalv1.MachineInventory, updated bool) (*elementalv1.MachineInventory, error) {
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
