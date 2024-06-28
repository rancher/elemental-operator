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
	"regexp"
	"strings"

	"github.com/gorilla/websocket"
	values "github.com/rancher/wrangler/v2/pkg/data"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/hostinfo"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/register"
)

type LegacyConfig struct {
	Elemental   elementalv1.Elemental  `yaml:"elemental"`
	CloudConfig map[string]interface{} `yaml:"cloud-config,omitempty"`
}

var (
	sanitize             = regexp.MustCompile("[^0-9a-zA-Z_]")
	sanitizeHostname     = regexp.MustCompile("[^0-9a-zA-Z.]")
	doubleDash           = regexp.MustCompile("--+")
	start                = regexp.MustCompile("^[a-zA-Z0-9]")
	errValueNotFound     = errors.New("value not found")
	errInventoryNotFound = errors.New("MachineInventory not found")
)

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
		if err := i.createIPAddressClaim(inventory, registration); err != nil {
			log.Errorf("cannot create the IP address claim to the IPAM provider: %s", err)
			return err
		}
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

func replaceStringData(data map[string]interface{}, name string) (string, error) {
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
		} else {
			return "", errValueNotFound
		}
		str = str[j+i+1:]
	}

	return result.String(), nil
}

func (i *InventoryServer) serveLoop(conn *websocket.Conn, inventory *elementalv1.MachineInventory, registration *elementalv1.MachineRegistration) error {
	protoVersion := register.MsgUndefined

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
			err = updateInventoryFromSMBIOSData(data, inventory, registration)
			if err != nil {
				return fmt.Errorf("failed to extract labels from SMBIOS data: %w", err)
			}
			log.Debugf("received SMBIOS data - generated machine name: %s", inventory.Name)
		case register.MsgLabels:
			if err := mergeInventoryLabels(inventory, data); err != nil {
				return err
			}
		case register.MsgAnnotations:
			err = updateInventoryWithAnnotations(data, inventory)
			if err != nil {
				return fmt.Errorf("failed to decode dynamic data: %w", err)
			}
		case register.MsgGet:
			return i.handleGet(conn, protoVersion, inventory, registration)
		case register.MsgUpdate:
			err = i.handleUpdate(conn, protoVersion, inventory)
			if err != nil {
				return fmt.Errorf("failed to negotiate registration update: %w", err)
			}
		case register.MsgSystemData:
			err = updateInventoryFromSystemData(data, inventory, registration)
			if err != nil {
				return fmt.Errorf("failed to extract labels from system data: %w", err)
			}
		case register.MsgSystemDataV2:
			err = updateInventoryFromSystemDataNG(data, inventory, registration)
			if err != nil {
				return fmt.Errorf("failed to extract labels from system data: %w", err)
			}
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

func updateInventoryWithAnnotations(data []byte, mInventory *elementalv1.MachineInventory) error {
	annotations := map[string]string{}
	if err := json.Unmarshal(data, &annotations); err != nil {
		return err
	}
	log.Debug("Adding annotations from client data")
	if mInventory.Annotations == nil {
		mInventory.Annotations = map[string]string{}
	}
	for key, val := range annotations {
		mInventory.Annotations[fmt.Sprintf("elemental.cattle.io/%s", sanitizeUserInput(key))] = sanitizeUserInput(val)
	}
	return nil
}

// updateInventoryFromSMBIOSData() updates mInventory Name and Labels from the MachineRegistration and the SMBIOS data
func updateInventoryFromSMBIOSData(data []byte, mInventory *elementalv1.MachineInventory, mRegistration *elementalv1.MachineRegistration) error {
	smbiosData := map[string]interface{}{}
	if err := json.Unmarshal(data, &smbiosData); err != nil {
		return err
	}
	// Sanitize any lower dashes into dashes as hostnames cannot have lower dashes, and we use the inventory name
	// to set the machine hostname. Also set it to lowercase
	name, err := replaceStringData(smbiosData, mInventory.Name)
	if err == nil {
		name = sanitizeStringHostname(name)
		mInventory.Name = strings.ToLower(sanitizeHostname.ReplaceAllString(name, "-"))
	} else {
		if errors.Is(err, errValueNotFound) {
			// value not found, will be set in updateInventoryFromSystemData
			log.Warningf("SMBIOS Value not found: %v", mInventory.Name)
		} else {
			return err
		}
	}

	log.Debugf("Adding labels from registration")
	// Add extra label info from data coming from smbios and based on the registration data
	if mInventory.Labels == nil {
		mInventory.Labels = map[string]string{}
	}
	for k, v := range mRegistration.Spec.MachineInventoryLabels {
		parsedData, err := replaceStringData(smbiosData, v)
		if err != nil {
			if errors.Is(err, errValueNotFound) {
				log.Debugf("Value not found: %v", v)
				continue
			}
			log.Errorf("Failed parsing smbios data: %v", err.Error())
			return err
		}
		parsedData = sanitizeString(parsedData)

		log.Debugf("Parsed %s into %s with smbios data, setting it to label %s", v, parsedData, k)
		mInventory.Labels[k] = strings.TrimSuffix(strings.TrimPrefix(parsedData, "-"), "-")
	}
	return nil
}

// updateInventoryFromSystemDataNG receives digested hardware labels from the client
func updateInventoryFromSystemDataNG(data []byte, inv *elementalv1.MachineInventory, reg *elementalv1.MachineRegistration) error {
	labels := map[string]interface{}{}

	if err := json.Unmarshal(data, &labels); err != nil {
		return fmt.Errorf("unmarshalling system data labels payload: %w", err)
	}

	return sanitizeSystemDataLabels(labels, inv, reg)
}

// Deprecated. Remove me together with 'MsgSystemData' type.
// updateInventoryFromSystemData creates labels in the inventory based on the hardware information
func updateInventoryFromSystemData(data []byte, inv *elementalv1.MachineInventory, reg *elementalv1.MachineRegistration) error {
	log.Infof("Adding labels from system data")

	labels, err := hostinfo.FillData(data)
	if err != nil {
		return err
	}

	return sanitizeSystemDataLabels(labels, inv, reg)
}

func sanitizeSystemDataLabels(labels map[string]interface{}, inv *elementalv1.MachineInventory, reg *elementalv1.MachineRegistration) error {
	// Also available but not used:
	// systemData.Product -> name, vendor, serial,uuid,sku,version. Kind of smbios data
	// systemData.BIOS -> info about the bios. Useless IMO
	// systemData.Baseboard -> asset, serial, vendor,version,product. Kind of useless?
	// systemData.Chassis -> asset, serial, vendor,version,product, type. Maybe be useful depending on the provider.
	// systemData.Topology -> CPU/memory and cache topology. No idea if useful.

	name, err := replaceStringData(labels, inv.Name)
	if err != nil {
		if errors.Is(err, errValueNotFound) {
			log.Warningf("System data value not found: %v", inv.Name)
			name = "m"
		} else {
			return err
		}
	}
	name = sanitizeStringHostname(name)

	inv.Name = strings.ToLower(sanitizeHostname.ReplaceAllString(name, "-"))

	log.Debugf("Parsing labels from System Data")

	if inv.Labels == nil {
		inv.Labels = map[string]string{}
	}

	for k, v := range reg.Spec.MachineInventoryLabels {
		log.Debugf("Parsing: %v : %v", k, v)

		parsedData, err := replaceStringData(labels, v)
		if err != nil {
			if errors.Is(err, errValueNotFound) {
				log.Debugf("Value not found: %v", v)
				continue
			}
			log.Errorf("Failed parsing system data: %v", err.Error())
			return err
		}
		parsedData = sanitizeString(parsedData)

		log.Debugf("Parsed %s into %s with system data, setting it to label %s", v, parsedData, k)
		inv.Labels[k] = strings.TrimSuffix(strings.TrimPrefix(parsedData, "-"), "-")
	}

	return nil
}

// sanitizeString will sanitize a given string by:
// replacing all invalid chars as set on the sanitize regex by dashes
// removing any double dashes resulted from the above method
// removing prefix+suffix if they are a dash
func sanitizeString(s string) string {
	s1 := sanitize.ReplaceAllString(s, "-")
	s2 := doubleDash.ReplaceAllString(s1, "-")
	if !start.MatchString(s2) {
		s2 = "m" + s2
	}
	if len(s2) > 58 {
		s2 = s2[:58]
	}
	return s2
}

// like sanitizeString but allows also '.' inside "s"
func sanitizeStringHostname(s string) string {
	s1 := sanitizeHostname.ReplaceAllString(s, "-")
	s2 := doubleDash.ReplaceAllLiteralString(s1, "-")
	if !start.MatchString(s2) {
		s2 = "m" + s2
	}
	if len(s2) > 58 {
		s2 = s2[:58]
	}
	return s2
}

func mergeInventoryLabels(inventory *elementalv1.MachineInventory, data []byte) error {
	labels := map[string]string{}
	if err := json.Unmarshal(data, &labels); err != nil {
		return fmt.Errorf("cannot extract inventory labels: %w", err)
	}
	log.Debugf("received labels: %v", labels)
	log.Warningf("received labels from registering client: no more supported, skipping")
	if inventory.Labels == nil {
		inventory.Labels = map[string]string{}
	}
	return nil
}

func isNewInventory(inventory *elementalv1.MachineInventory) bool {
	return inventory.CreationTimestamp.IsZero()
}
