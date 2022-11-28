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

func (i *InventoryServer) unauthenticatedResponse(registration *elementalv1.MachineRegistration, writer io.Writer) error {
	if registration.Spec.Config == nil {
		registration.Spec.Config = &elementalv1.Config{}
	}

	mRegistration := registration.Spec.Config.Elemental.Registration

	return yaml.NewEncoder(writer).Encode(elementalv1.Config{
		Elemental: elementalv1.Elemental{
			Registration: elementalv1.Registration{
				URL:             registration.Status.RegistrationURL,
				CACert:          i.getRancherCACert(),
				EmulateTPM:      mRegistration.EmulateTPM,
				EmulatedTPMSeed: mRegistration.EmulatedTPMSeed,
				NoSMBIOS:        mRegistration.NoSMBIOS,
			},
		},
	})
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

func (i *InventoryServer) getMachineRegistration(req *http.Request) (*elementalv1.MachineRegistration, error) {
	token := path.Base(req.URL.Path)

	mRegistrationList := &elementalv1.MachineRegistrationList{}
	if err := i.List(i, mRegistrationList); err != nil {
		return nil, fmt.Errorf("failed to list machine registrations")
	}

	var mRegistration *elementalv1.MachineRegistration

	// TODO: build machine registrations cache indexed by token
	for _, m := range mRegistrationList.Items {
		if m.Status.RegistrationToken == token {
			// Found two registrations with the same registration token
			if mRegistration != nil {
				return nil, fmt.Errorf("machine registrations %s/%s and %s/%s have the same registration token %s",
					mRegistration.Namespace, mRegistration.Name, m.Namespace, m.Name, token)
			}
			mRegistration = &m
		}
	}

	if mRegistration == nil {
		return nil, fmt.Errorf("failed to find machine registration with registration token %s", token)
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

func (i *InventoryServer) writeMachineInventoryCloudConfig(conn *websocket.Conn, inventory *elementalv1.MachineInventory, registration *elementalv1.MachineRegistration) error {
	sa := &corev1.ServiceAccount{}

	if err := i.Get(i, types.NamespacedName{
		Name:      registration.Status.ServiceAccountRef.Name,
		Namespace: registration.Status.ServiceAccountRef.Namespace,
	}, sa); err != nil {
		return fmt.Errorf("failed to get service account: %w", err)
	}

	if len(sa.Secrets) == 0 {
		return fmt.Errorf("no secrets associated to the %s/%s service account", sa.Namespace, sa.Name)
	}

	secret := &corev1.Secret{}
	err := i.Get(i, types.NamespacedName{
		Name:      sa.Secrets[0].Name,
		Namespace: sa.Namespace,
	}, secret)

	if err != nil || secret.Type != corev1.SecretTypeServiceAccountToken {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	serverURL, err := i.getRancherServerURL()
	if err != nil {
		return fmt.Errorf("failed to get server-url: %w", err)
	}

	writer, err := conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return err
	}
	defer writer.Close()

	if registration.Spec.Config == nil {
		registration.Spec.Config = &elementalv1.Config{}
	}

	return yaml.NewEncoder(writer).Encode(elementalv1.Config{
		Elemental: elementalv1.Elemental{
			Registration: elementalv1.Registration{
				URL:    registration.Status.RegistrationURL,
				CACert: i.getRancherCACert(),
			},
			SystemAgent: elementalv1.SystemAgent{
				URL:             fmt.Sprintf("%s/k8s/clusters/local", serverURL),
				Token:           string(secret.Data["token"]),
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

func (i *InventoryServer) serveLoop(conn *websocket.Conn, inventory *elementalv1.MachineInventory, registration *elementalv1.MachineRegistration) (msgType register.MessageType, err error) {
	updated := false
	protoVersion := register.MsgUndefined

	for {
		var data []byte

		msgType, data, err = register.ReadMessage(conn)
		if err != nil {
			return msgType, fmt.Errorf("websocket communication interrupted: %w", err)
		}
		replyMsgType := register.MsgReady
		replyData := []byte{}

		switch msgType {
		case register.MsgVersion:
			if len(data) != 1 {
				return msgType, fmt.Errorf("failed to negotiate the protocol version, got: %v (%s)", data, data)
			}
			clientVersion := register.MessageType(data[0])
			protoVersion = clientVersion
			if clientVersion != register.MsgLast {
				logrus.Infof("elemental-register (%d) and elemental-operator (%d) protocol versions differ", clientVersion, register.MsgLast)
				if clientVersion > register.MsgLast {
					protoVersion = register.MsgLast
				}
			}

			logrus.Infof("Negotiated protocol version: %d", protoVersion)
			replyMsgType = register.MsgVersion
			replyData = []byte{byte(protoVersion)}
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
			for k, v := range registration.Spec.MachineInventoryLabels {
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
			err = i.writeMachineInventoryCloudConfig(conn, inventory, registration)
			if err != nil {
				return msgType, fmt.Errorf("failed sending elemental cloud config: %w", err)
			}
			logrus.Debug("elemental cloud config sent")
			if protoVersion == register.MsgUndefined {
				logrus.Warn("Detected old register client: cloud-config data may not be applied correctly")
			}
			return msgType, nil

		default:
			return msgType, fmt.Errorf("got unexpected message: %s", msgType)
		}
		if err := register.WriteMessage(conn, replyMsgType, replyData); err != nil {
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
