/*
Copyright © 2023 SUSE LLC

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
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	values "github.com/rancher/wrangler/pkg/data"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/hostinfo"
	"github.com/rancher/elemental-operator/pkg/register"
)

var (
	sanitize         = regexp.MustCompile("[^0-9a-zA-Z_]")
	sanitizeHostname = regexp.MustCompile("[^0-9a-zA-Z]")
	doubleDash       = regexp.MustCompile("--+")
	start            = regexp.MustCompile("^[a-zA-Z0-9]")
)

func (i *InventoryServer) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	escapedPath := strings.Replace(req.URL.Path, "\n", "", -1)
	escapedPath = strings.Replace(escapedPath, "\r", "", -1)
	logrus.Infof("Incoming HTTP request for %s", escapedPath)

	splittedPath := strings.Split(escapedPath, "/")
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

func (i *InventoryServer) apiBuildImage(resp http.ResponseWriter, req *http.Request) error {
	logrus.Debug("apiBuildImage")
	return nil
}

func (i *InventoryServer) apiRegistration(resp http.ResponseWriter, req *http.Request) error {
	var err error
	var registration *elementalv1.MachineRegistration

	// get the machine registration relevant to this request
	if registration, err = i.getMachineRegistration(req); err != nil {
		http.Error(resp, err.Error(), http.StatusNotFound)
		return err
	}

	if !websocket.IsWebSocketUpgrade(req) {
		logrus.Debug("got a plain HTTP request: send unauthenticated registration")
		if err = i.unauthenticatedResponse(registration, resp); err != nil {
			logrus.Error("error sending unauthenticated response: ", err)
		}
		return err
	}

	// upgrade to websocket
	conn, err := upgrade(resp, req)
	if err != nil {
		logrus.Error("failed to upgrade connection to websocket: %w", err)
		return err
	}
	defer conn.Close()
	logrus.Debug("connection upgraded to websocket")

	// attempt to authenticate the machine, if err, authentication has failed
	inventory, err := i.authMachine(conn, req, registration.Namespace)
	if err != nil {
		logrus.Error("authentication failed: ", err)
		return err
	}
	// no error and no inventory: Auth header is missing or unrecognized
	if inventory == nil {
		logrus.Warn("websocket connection: unrecognized authentication method")
		if err = i.unauthenticatedResponse(registration, resp); err != nil {
			logrus.Error("error sending unauthenticated response: ", err)
		}
		return err
	}
	logrus.Debug("attestation completed")

	if err = register.WriteMessage(conn, register.MsgReady, []byte{}); err != nil {
		logrus.Error("cannot finalize the authentication process: %w", err)
		return err
	}

	isNewInventory := inventory.CreationTimestamp.IsZero()
	if isNewInventory {
		initInventory(inventory, registration)
	}

	if err = i.serveLoop(conn, inventory, registration); err != nil {
		logrus.Error(err)
		return err
	}

	return nil
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

func (i *InventoryServer) writeMachineInventoryCloudConfig(conn *websocket.Conn, protoVersion register.MessageType, inventory *elementalv1.MachineInventory, registration *elementalv1.MachineRegistration) error {
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

	if registration.Spec.Config == nil {
		registration.Spec.Config = &elementalv1.Config{}
	}

	config := elementalv1.Config{
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
	}

	// If client does not support MsgConfig we send back the config as a
	// byte-stream without a message-type to keep backwards-compatibility.
	if protoVersion < register.MsgConfig {
		logrus.Debug("Client does not support MsgConfig, sending back raw config.")

		writer, err := conn.NextWriter(websocket.BinaryMessage)
		if err != nil {
			return err
		}
		defer writer.Close()

		return yaml.NewEncoder(writer).Encode(config)
	}

	logrus.Debug("Client supports MsgConfig, sending back config.")

	// If the client supports the MsgConfig-message we use that.
	data, err := yaml.Marshal(config)
	if err != nil {
		logrus.Errorf("error marshalling config: %v", err)
		return err
	}

	return register.WriteMessage(conn, register.MsgConfig, data)
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
				return fmt.Errorf("failed to negotiate protol version: %w", err)
			}
			logrus.Infof("Negotiated protocol version: %d", protoVersion)
			replyMsgType = register.MsgVersion
			replyData = []byte{byte(protoVersion)}
		case register.MsgSmbios:
			err = updateInventoryFromSMBIOSData(data, inventory, registration)
			if err != nil {
				return fmt.Errorf("failed to extract labels from SMBIOS data: %w", err)
			}
			logrus.Debugf("received SMBIOS data - generated machine name: %s", inventory.Name)
		case register.MsgLabels:
			if err := mergeInventoryLabels(inventory, data); err != nil {
				return err
			}
		case register.MsgGet:
			return i.handleGet(conn, protoVersion, inventory, registration)
		case register.MsgSystemData:
			err = updateInventoryFromSystemData(data, inventory)
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

func (i *InventoryServer) handleGet(conn *websocket.Conn, protoVersion register.MessageType, inventory *elementalv1.MachineInventory, registration *elementalv1.MachineRegistration) error {
	var err error

	inventory, err = i.commitMachineInventory(inventory)
	if err != nil {
		if writeErr := writeError(conn, protoVersion, register.NewErrorMessage(err)); writeErr != nil {
			logrus.Errorf("Error reporting back error to client: %v", writeErr)
		}

		return err
	}

	if err := i.writeMachineInventoryCloudConfig(conn, protoVersion, inventory, registration); err != nil {
		if writeErr := writeError(conn, protoVersion, register.NewErrorMessage(err)); writeErr != nil {
			logrus.Errorf("Error reporting back error to client: %v", writeErr)
		}

		return fmt.Errorf("failed sending elemental cloud config: %w", err)
	}

	logrus.Debug("Elemental cloud config sent")

	if protoVersion == register.MsgUndefined {
		logrus.Warn("Detected old elemental-register client: cloud-config data may not be applied correctly")
	}

	return nil
}

// writeError reports back an error to the client if the negotiated protocol
// version supports it.
func writeError(conn *websocket.Conn, protoVersion register.MessageType, errorMessage register.ErrorMessage) error {
	if protoVersion < register.MsgError {
		logrus.Debugf("client does not support reporting errors, skipping")
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
		logrus.Debugf("elemental-register (%d) and elemental-operator (%d) protocol versions differ", clientVersion, register.MsgLast)
		if clientVersion > register.MsgLast {
			protoVersion = register.MsgLast
		}
	}

	return protoVersion, nil
}

// updateInventoryFromSMBIOSData() updates mInventory Name and Labels from the MachineRegistration and the SMBIOS data
func updateInventoryFromSMBIOSData(data []byte, mInventory *elementalv1.MachineInventory, mRegistration *elementalv1.MachineRegistration) error {
	smbiosData := map[string]interface{}{}
	if err := json.Unmarshal(data, &smbiosData); err != nil {
		return err
	}
	// Sanitize any lower dashes into dashes as hostnames cannot have lower dashes, and we use the inventory name
	// to set the machine hostname. Also set it to lowercase
	mInventory.Name = strings.ToLower(sanitizeHostname.ReplaceAllString(buildStringFromSmbiosData(smbiosData, mInventory.Name), "-"))
	logrus.Debug("Adding labels from registration")
	// Add extra label info from data coming from smbios and based on the registration data
	if mInventory.Labels == nil {
		mInventory.Labels = map[string]string{}
	}
	for k, v := range mRegistration.Spec.MachineInventoryLabels {
		parsedData := buildStringFromSmbiosData(smbiosData, v)
		logrus.Debugf("Parsed %s into %s with smbios data, setting it to label %s", v, parsedData, k)
		mInventory.Labels[k] = strings.TrimSuffix(strings.TrimPrefix(parsedData, "-"), "-")
	}
	return nil
}

// updateInventoryFromSystemData creates labels in the inventory based on the hardware information
func updateInventoryFromSystemData(data []byte, inv *elementalv1.MachineInventory) error {
	systemData := &hostinfo.HostInfo{}
	if err := json.Unmarshal(data, &systemData); err != nil {
		return err
	}
	logrus.Info("Adding labels from system data")
	if inv.Labels == nil {
		inv.Labels = map[string]string{}
	}
	inv.Labels["elemental.cattle.io/TotalMemory"] = strconv.Itoa(int(systemData.Memory.TotalPhysicalBytes))

	// Both checks below is due to ghw not detecting aarch64 cores/threads properly, so it ends up in a label
	// with 0 valuie, which is not useful at all
	// tracking bug: https://github.com/jaypipes/ghw/issues/199
	if systemData.CPU.TotalCores > 0 {
		inv.Labels["elemental.cattle.io/CpuTotalCores"] = strconv.Itoa(int(systemData.CPU.TotalCores))
	}

	if systemData.CPU.TotalThreads > 0 {
		inv.Labels["elemental.cattle.io/CpuTotalThreads"] = strconv.Itoa(int(systemData.CPU.TotalThreads))
	}

	// This should never happen but just in case
	if len(systemData.CPU.Processors) > 0 {
		// Model still looks weird, maybe there is a way of getting it differently as we need to sanitize a lot of data in there?
		// Currently, something like "Intel(R) Core(TM) i7-7700K CPU @ 4.20GHz" ends up being:
		// "Intel-R-Core-TM-i7-7700K-CPU-4-20GHz"
		inv.Labels["elemental.cattle.io/CpuModel"] = sanitizeString(systemData.CPU.Processors[0].Model)
		inv.Labels["elemental.cattle.io/CpuVendor"] = sanitizeString(systemData.CPU.Processors[0].Vendor)
		// Capabilities available here at systemData.CPU.Processors[X].Capabilities
	}
	// This could happen so always check.
	if systemData.GPU != nil && len(systemData.GPU.GraphicsCards) > 0 && systemData.GPU.GraphicsCards[0].DeviceInfo != nil {
		inv.Labels["elemental.cattle.io/GpuModel"] = sanitizeString(systemData.GPU.GraphicsCards[0].DeviceInfo.Product.Name)
		inv.Labels["elemental.cattle.io/GpuVendor"] = sanitizeString(systemData.GPU.GraphicsCards[0].DeviceInfo.Vendor.Name)
	}
	inv.Labels["elemental.cattle.io/NetNumberInterfaces"] = strconv.Itoa(len(systemData.Network.NICs))

	for index, iface := range systemData.Network.NICs {
		inv.Labels[fmt.Sprintf("elemental.cattle.io/NetIface%d-Name", index)] = iface.Name
		// inv.Labels[fmt.Sprintf("NetIface%d-MAC", index)] = base64.StdEncoding.EncodeToString([]byte(iface.MacAddress)) // Could work encoded but...kind of crappy
		inv.Labels[fmt.Sprintf("elemental.cattle.io/NetIface%d-Virtual", index)] = strconv.FormatBool(iface.IsVirtual)
		// Capabilities available here at iface.Capabilities
		// interesting to store anything in here or show it on the docs? Difficult to use it afterwards as its a list...
	}
	inv.Labels["BlockTotal"] = strconv.Itoa(len(systemData.Block.Disks)) // This includes removable devices like cdrom/usb
	for index, block := range systemData.Block.Disks {
		inv.Labels[fmt.Sprintf("elemental.cattle.io/BlockDevice%d-Size", index)] = strconv.Itoa(int(block.SizeBytes))
		inv.Labels[fmt.Sprintf("elemental.cattle.io/BlockDevice%d-Name", index)] = block.Name
		inv.Labels[fmt.Sprintf("elemental.cattle.io/BlockDevice%d-DriveType", index)] = block.DriveType.String()
		inv.Labels[fmt.Sprintf("elemental.cattle.io/BlockDevice%d-ControllerType", index)] = block.StorageController.String()
		inv.Labels[fmt.Sprintf("elemental.cattle.io/BlockDevice%d-Removable", index)] = strconv.FormatBool(block.IsRemovable)
		// Vendor and model also available here, useful?
	}

	// Also available but not used:
	// systemData.Product -> name, vendor, serial,uuid,sku,version. Kind of smbios data
	// systemData.BIOS -> info about the bios. Useless IMO
	// systemData.Baseboard -> asset, serial, vendor,version,product. Kind of useless?
	// systemData.Chassis -> asset, serial, vendor,version,product, type. Maybe be useful depending on the provider.
	// systemData.Topology -> CPU/memory and cache topology. No idea if useful.

	return nil
}

// sanitizeString will sanitize a given string by:
// replacing all invalid chars as set on the sanitize regex by dashes
// removing any double dashes resulted from the above method
// removing prefix+suffix if they are a dash
func sanitizeString(s string) string {
	s1 := sanitize.ReplaceAllString(s, "-")
	s2 := doubleDash.ReplaceAllString(s1, "-")
	return strings.TrimSuffix(strings.TrimPrefix(s2, "-"), "-")
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
