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
	"fmt"
	"regexp"
	"strings"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/templater"
)

var (
	sanitize         = regexp.MustCompile("[^0-9a-zA-Z_]")
	sanitizeHostname = regexp.MustCompile("[^0-9a-zA-Z.]")
	doubleDash       = regexp.MustCompile("--+")
	start            = regexp.MustCompile("^[a-zA-Z0-9]")
)

func updateInventoryName(tmpl templater.Templater, inv *elementalv1.MachineInventory) error {
	// Inventory Name should be set only on freshly created inventories.
	if !isNewInventory(inv) {
		return nil
	}
	// Sanitize any lower dashes into dashes as hostnames cannot have lower dashes, and we use the inventory name
	// to set the machine hostname. Also set it to lowercase.
	name, err := tmpl.Decode(inv.Name)
	if err != nil {
		if templater.IsValueNotFoundError(err) {
			name = generateInventoryName()
			log.Warningf("Templater cannot decode MachineInventory name %q, fallback to %q.", inv.Name, name)
			inv.Name = name
			return nil
		}
		return fmt.Errorf("templater: cannot decode MachineInventory name %q: %w", inv.Name, err)
	}
	name = sanitizeStringHostname(name)

	// Something went wrong, decoding and sanitizing the hostname it got empty.
	if name == "" {
		return fmt.Errorf("invalid MachineInventory name: %q", name)
	}
	inv.Name = strings.ToLower(sanitizeHostname.ReplaceAllString(name, "-"))
	return nil
}

func updateInventoryLabels(tmpl templater.Templater, inv *elementalv1.MachineInventory, reg *elementalv1.MachineRegistration) error {
	log.Debugf("Adding registration labels")
	if inv.Labels == nil {
		inv.Labels = map[string]string{}
	}
	for k, v := range reg.Spec.MachineInventoryLabels {
		decodedLabel, err := tmpl.Decode(v)
		if err != nil {
			if templater.IsValueNotFoundError(err) {
				log.Warningf("Templater cannot decode label '%q': %s", v, err.Error())
				continue
			}
			log.Errorf("Templater failed decoding label '%q': %s", v, err.Error())
			return err
		}
		decodedLabel = sanitizeString(decodedLabel)

		log.Debugf("Decoded %s into %s, setting it to label %s", v, decodedLabel, k)
		inv.Labels[k] = strings.TrimSuffix(strings.TrimPrefix(decodedLabel, "-"), "-")
	}
	return nil
}

func updateInventoryWithLabels(inventory *elementalv1.MachineInventory, data []byte) error {
	labels := map[string]string{}
	if err := json.Unmarshal(data, &labels); err != nil {
		return fmt.Errorf("cannot extract inventory labels: %w", err)
	}
	log.Debugf("received labels: %v", labels)
	log.Errorf("received labels from registering client: no more supported, skipping")
	if inventory.Labels == nil {
		inventory.Labels = map[string]string{}
	}
	return nil
}

func updateInventoryWithAnnotations(data []byte, mInventory *elementalv1.MachineInventory) error {
	annotations := map[string]string{}
	if err := json.Unmarshal(data, &annotations); err != nil {
		return fmt.Errorf("cannot extract inventory annotations: %w", err)
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

// like sanitizeString but allows also '.' inside "s"
func sanitizeStringHostname(s string) string {
	if s == "" {
		return ""
	}
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

func isNewInventory(inventory *elementalv1.MachineInventory) bool {
	return inventory.CreationTimestamp.IsZero()
}
