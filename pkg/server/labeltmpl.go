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
	"strings"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/hostinfo"
	"github.com/rancher/elemental-operator/pkg/log"
	values "github.com/rancher/wrangler/v2/pkg/data"
)

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
