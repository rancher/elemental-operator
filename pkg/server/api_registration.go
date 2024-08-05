/*
Copyright © 2022 - 2024 SUSE LLC

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

	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/hostinfo"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/register"
	"github.com/rancher/elemental-operator/pkg/templater"
)

type LegacyConfig struct {
	Elemental   elementalv1.Elemental  `yaml:"elemental"`
	CloudConfig map[string]interface{} `yaml:"cloud-config,omitempty"`
}

var errInventoryNotFound = errors.New("MachineInventory not found")

func (i *InventoryServer) apiRegistration(resp http.ResponseWriter, req *http.Request) error {
	var err error
	var registration *elementalv1.MachineRegistration

	// get the machine registration relevant to this request
	if registration, err = i.getMachineRegistration(path.Base(req.URL.Path)); err != nil {
		http.Error(resp, err.Error(), http.StatusNotFound)
		return err
	}

	if !websocket.IsWebSocketUpgrade(req) {
		log.Debugf("got a plain HTTP request: send unauthenticated registration")
		if err = i.unauthenticatedResponse(registration, resp); err != nil {
			log.Errorf("error sending unauthenticated response: %s", err)
		}
		return err
	}

	// upgrade to websocket
	conn, err := upgrade(resp, req)
	if err != nil {
		log.Errorf("failed to upgrade connection to websocket: %s", err)
		return err
	}
	defer conn.Close()
	log.Debugf("connection upgraded to websocket")

	// attempt to authenticate the machine, if err, authentication has failed
	inventory, err := i.authMachine(conn, req, registration.Namespace)
	if err != nil {
		log.Errorf("authentication failed: %s", err)
		http.Error(resp, "authentication failure", http.StatusUnauthorized)
		return err
	}
	// no error and no inventory: Auth header is missing or unrecognized
	if inventory == nil {
		if authHeader := req.Header.Get("Authorization"); authHeader != "" {
			errMsg := "unrecognized authentication method"
			log.Errorf("websocket connection: %s", errMsg)
			http.Error(resp, errMsg, http.StatusUnauthorized)
			return err
		}

		log.Warning("unauthenticated websocket connection")
		if err = i.unauthenticatedResponse(registration, resp); err != nil {
			log.Errorf("error sending unauthenticated response: %s", err)
			return err
		}
		return nil
	}
	log.Debugf("authentication completed")

	if err = register.WriteMessage(conn, register.MsgReady, []byte{}); err != nil {
		log.Errorf("cannot finalize the authentication process: %s", err)
		return err
	}

	if isNewInventory(inventory) {
		initInventory(inventory, registration)
	}

	if err = i.serveLoop(conn, inventory, registration); err != nil {
		log.Errorf("Error during serve-loop: %s", err)
		return err
	}

	return nil
}

func (i *InventoryServer) unauthenticatedResponse(registration *elementalv1.MachineRegistration, writer io.Writer) error {
	config, err := registration.GetClientRegistrationConfig(i.getRancherCACert())
	if err != nil {
		return err
	}
	return yaml.NewEncoder(writer).
		Encode(config)
}

func (i *InventoryServer) writeMachineInventoryCloudConfig(conn *websocket.Conn, protoVersion register.MessageType, inventory *elementalv1.MachineInventory, registration *elementalv1.MachineRegistration) error {
	sa := &corev1.ServiceAccount{}

	if err := i.Get(i, types.NamespacedName{
		Name:      registration.Status.ServiceAccountRef.Name,
		Namespace: registration.Status.ServiceAccountRef.Namespace,
	}, sa); err != nil {
		return fmt.Errorf("failed to get service account: %w", err)
	}

	secret := &corev1.Secret{}
	err := i.Get(i, types.NamespacedName{
		Name:      sa.Name + elementalv1.SASecretSuffix,
		Namespace: sa.Namespace,
	}, secret)

	if err != nil || secret.Type != corev1.SecretTypeServiceAccountToken {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	serverURL, err := i.getValue("server-url")
	if err != nil {
		return fmt.Errorf("failed to get server-url: %w", err)
	}
	if serverURL == "" {
		return fmt.Errorf("server-url is not set")
	}

	if registration.Spec.Config == nil {
		registration.Spec.Config = &elementalv1.Config{}
	}

	config, err := registration.GetClientRegistrationConfig(i.getRancherCACert())
	if err != nil {
		return err
	}
	config.Elemental.SystemAgent = elementalv1.SystemAgent{
		URL:             fmt.Sprintf("%s/k8s/clusters/local", serverURL),
		Token:           string(secret.Data["token"]),
		SecretName:      inventory.Name,
		SecretNamespace: inventory.Namespace,
	}
	config.Elemental.Install = registration.Spec.Config.Elemental.Install
	config.Elemental.Reset = registration.Spec.Config.Elemental.Reset
	config.CloudConfig = registration.Spec.Config.CloudConfig

	// If client does not support MsgConfig we send back the config as a
	// byte-stream without a message-type to keep backwards-compatibility.
	if protoVersion < register.MsgConfig {
		log.Debugf("Client does not support MsgConfig, sending back raw config.")

		writer, err := conn.NextWriter(websocket.BinaryMessage)
		if err != nil {
			return err
		}
		defer writer.Close()

		return yaml.NewEncoder(writer).Encode(config)
	}

	log.Debugf("Client supports MsgConfig, sending back config.")

	// If the client supports the MsgConfig-message we use that.
	data, err := yaml.Marshal(config)
	if err != nil {
		log.Errorf("error marshalling config: %v", err)
		return err
	}

	return register.WriteMessage(conn, register.MsgConfig, data)
}

func (i *InventoryServer) getRancherCACert() string {
	cacert, err := i.getValue("cacerts")
	if err != nil {
		log.Errorf("Error getting cacerts: %s", err.Error())
	}

	if cacert == "" {
		if cacert, err = i.getValue("internal-cacerts"); err != nil {
			log.Errorf("Error getting internal-cacerts: %s", err.Error())
			return ""
		}
	}
	return cacert
}

func (i *InventoryServer) serveLoop(conn *websocket.Conn, inventory *elementalv1.MachineInventory, registration *elementalv1.MachineRegistration) error {
	protoVersion := register.MsgUndefined
	tmpl := templater.NewTemplater()

	for {
		var data []byte

		msgType, data, err := register.ReadMessage(conn)
		if err != nil {
			return fmt.Errorf("websocket communication interrupted: %w", err)
		}
		replyMsgType := register.MsgReady
		replyData := []byte{}

		switch msgType {
		case register.MsgVersion:
			protoVersion, err = decodeProtocolVersion(data)
			if err != nil {
				return fmt.Errorf("failed to negotiate protocol version: %w", err)
			}
			log.Infof("Negotiated protocol version: %d", protoVersion)
			replyMsgType = register.MsgVersion
			replyData = []byte{byte(protoVersion)}
		case register.MsgSmbios:
			smbiosData := map[string]interface{}{}
			if err := json.Unmarshal(data, &smbiosData); err != nil {
				return fmt.Errorf("failed to extract labels from SMBIOS data: %w", err)
			}
			tmpl.Fill(smbiosData)
		case register.MsgLabels:
			if err := mergeInventoryLabels(inventory, data); err != nil {
				return err
			}
		case register.MsgAnnotations:
			err = mergeInventoryAnnotations(data, inventory)
			if err != nil {
				return fmt.Errorf("failed to decode dynamic data: %w", err)
			}
		case register.MsgGet:
			// Final call: here we commit the MachineInventory, send the Elemental config data
			// and close the connection.
			if err := updateInventoryWithTemplates(tmpl, inventory, registration); err != nil {
				return err
			}
			return i.handleGet(conn, protoVersion, inventory, registration)
		case register.MsgUpdate:
			err = i.handleUpdate(conn, protoVersion, inventory)
			if err != nil {
				return fmt.Errorf("failed to negotiate registration update: %w", err)
			}
		case register.MsgSystemData:
			systemData, err := hostinfo.FillData(data)
			if err != nil {
				return fmt.Errorf("failed to parse system data: %w", err)
			}
			tmpl.Fill(systemData)
		case register.MsgSystemDataV2:
			systemData := map[string]interface{}{}
			if err := json.Unmarshal(data, &systemData); err != nil {
				return fmt.Errorf("failed to parse system data: %w", err)
			}
			tmpl.Fill(systemData)
		default:
			return fmt.Errorf("got unexpected message: %s", msgType)
		}
		if err := register.WriteMessage(conn, replyMsgType, replyData); err != nil {
			return fmt.Errorf("cannot complete %s exchange", msgType)
		}
	}
}

func (i *InventoryServer) handleUpdate(conn *websocket.Conn, protoVersion register.MessageType, inventory *elementalv1.MachineInventory) error {
	if isNewInventory(inventory) {
		log.Errorf("MachineInventory '%+v' was not found, but the machine is still running. Reprovisioning is needed.\n", inventory)
		if writeErr := writeError(conn, protoVersion, register.NewErrorMessage(errInventoryNotFound)); writeErr != nil {
			log.Errorf("Error reporting back error to client: %v\n", writeErr)
		}
		return errInventoryNotFound
	}
	return nil
}

func (i *InventoryServer) handleGet(conn *websocket.Conn, protoVersion register.MessageType, inventory *elementalv1.MachineInventory, registration *elementalv1.MachineRegistration) error {
	var err error

	inventory, err = i.commitMachineInventory(inventory)
	if err != nil {
		if writeErr := writeError(conn, protoVersion, register.NewErrorMessage(err)); writeErr != nil {
			log.Errorf("Error reporting back error to client: %v", writeErr)
		}

		return err
	}

	if err := i.writeMachineInventoryCloudConfig(conn, protoVersion, inventory, registration); err != nil {
		if writeErr := writeError(conn, protoVersion, register.NewErrorMessage(err)); writeErr != nil {
			log.Errorf("Error reporting back error to client: %v", writeErr)
		}

		return fmt.Errorf("failed sending elemental cloud config: %w", err)
	}

	log.Debugf("Elemental cloud config sent")

	return nil
}

// writeError reports back an error to the client if the negotiated protocol
// version supports it.
func writeError(conn *websocket.Conn, protoVersion register.MessageType, errorMessage register.ErrorMessage) error {
	if protoVersion < register.MsgError {
		log.Debugf("client does not support reporting errors, skipping")
		return nil
	}

	data, err := yaml.Marshal(errorMessage)
	if err != nil {
		return fmt.Errorf("error marshalling error-message: %w", err)
	}

	return register.WriteMessage(conn, register.MsgError, data)
}

func decodeProtocolVersion(data []byte) (register.MessageType, error) {
	protoVersion := register.MsgUndefined

	if len(data) != 1 {
		return protoVersion, fmt.Errorf("unknown format: %v (%s)", data, data)
	}
	clientVersion := register.MessageType(data[0])
	protoVersion = clientVersion
	if clientVersion != register.MsgLast {
		log.Debugf("elemental-register (%d) and elemental-operator (%d) protocol versions differ", clientVersion, register.MsgLast)
		if clientVersion <= register.MsgGet {
			return protoVersion, fmt.Errorf("elemental-register protocol version is deprecated (elemental-register client too old, protocol version = %d)", clientVersion)
		}
		if clientVersion > register.MsgLast {
			protoVersion = register.MsgLast
		}
	}

	return protoVersion, nil
}
