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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	elm "github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/config"
	values "github.com/rancher/wrangler/pkg/data"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const defaultName = "m-${System Information/Manufacturer}-${System Information/Product Name}-${System Information/UUID}"

var (
	sanitize   = regexp.MustCompile("[^0-9a-zA-Z]")
	doubleDash = regexp.MustCompile("--+")
	start      = regexp.MustCompile("^[a-zA-Z]")
)

func (i *InventoryServer) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	logrus.Debugf("Incoming HTTP request for %s", req.URL.Path)
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

	// attempt to authenticate the machine, if err, authentication has failed
	inventory, err := i.authMachine(conn, req, registration.Namespace)
	if err != nil {
		logrus.Error("failed to authenticate inventory: ", err)
		return
	}
	// no error and no inventory: Auth header is missing or unrecognized
	if inventory == nil {
		logrus.Info("websocket connection without recognized Auth header")
		if err = i.unauthenticatedResponse(registration, resp); err != nil {
			logrus.Error("error sending unauthenticated response: ", err)
		}
		return
	}

	if inventory.CreationTimestamp.IsZero() {
		inventory, err = i.createMachineInventory(req, inventory, registration)
		if err != nil {
			logrus.Error("error creating machine inventory: ", err)
			return
		}
		logrus.Infof("new machine inventory created: %s", inventory.Name)
	}

	labels, err := getLabels(req)
	if err != nil {
		logrus.Warn("failed to parse labels header: ", err)
	}

	if len(labels) > 0 {
		if inventory.Labels == nil {
			inventory.Labels = labels
		} else {
			for k, v := range labels {
				inventory.Labels[k] = v
			}
		}
		inventory, err = i.machineClient.Update(inventory)
		if err != nil {
			logrus.Error("failed to update inventory labels: ", err)
			return
		}
	}

	err = i.writeMachineInventoryCloudConfig(conn, inventory, registration)
	if err != nil {
		logrus.Error("failed sending elemental cloud config: %w", err)
		return
	}
	logrus.Debug("elemental cloud config sent")
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
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))

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
				Labels:          mRRegistration.Labels,
			},
		},
	})
}

func (i *InventoryServer) createMachineInventory(req *http.Request, inventory *elm.MachineInventory, registration *elm.MachineRegistration) (*elm.MachineInventory, error) {
	inventory.Name = registration.Spec.MachineName
	if inventory.Name == "" {
		inventory.Name = defaultName
	}
	sMBios, _ := getSMBios(req)
	inventory.Name = buildName(sMBios, inventory.Name)
	inventory.Namespace = registration.Namespace
	inventory.Annotations = registration.Spec.MachineInventoryAnnotations
	inventory.Labels = registration.Spec.MachineInventoryLabels

	machines, err := i.machineCache.GetByIndex(tpmHashIndex, inventory.Spec.TPMHash)
	if err != nil || len(machines) > 0 {
		return nil, err
	}

	return i.machineClient.Create(inventory)
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

func buildName(data map[string]interface{}, name string) string {
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
	return strings.ToLower(resultStr)
}

func getSMBios(req *http.Request) (map[string]interface{}, error) {
	var smbios string
	// Old header sent by clients on commit < be788bcfd899977770d84c996abd967c30942822
	headerOld := req.Header.Get("X-Cattle-Smbios")

	// If old header not found try to get the new ones
	if headerOld == "" {
		// 200 * 875bytes per header = 175Kb of smbios data, should be enough?
		for i := 1; i <= 200; i++ {
			header := req.Header.Get(fmt.Sprintf("X-Cattle-Smbios-%d", i))
			if header == "" {
				break
			}
			smbios = smbios + header
		}
	} else {
		smbios = headerOld
	}

	if smbios == "" {
		logrus.Debug("No smbios headers")
		return nil, nil
	}

	smbiosData, err := base64.StdEncoding.DecodeString(smbios)
	if err != nil {
		logrus.Error("Error decoding smbios string")
		return nil, err
	}
	data := map[string]interface{}{}
	return data, json.Unmarshal(smbiosData, &data)
}

func getLabels(req *http.Request) (map[string]string, error) {
	in := req.Header.Get("X-Cattle-Labels")
	if in == "" {
		return nil, nil
	}

	labelString, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return nil, err
	}
	var labels map[string]string
	return labels, json.Unmarshal(labelString, &labels)
}
