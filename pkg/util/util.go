/*
Copyright © 2022 - 2023 SUSE LLC

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

package util

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rancher/elemental-operator/pkg/log"
)

func RemoveInvalidConditions(conditions []metav1.Condition) []metav1.Condition {
	newConditions := []metav1.Condition{}
	for _, cond := range conditions {
		if cond.Type == "" || cond.Status == "" || cond.LastTransitionTime.IsZero() || cond.Reason == "" {
			continue
		}
		newConditions = append(newConditions, cond)
	}
	return newConditions
}

func MarshalCloudConfig(cloudConfig map[string]runtime.RawExtension) ([]byte, error) {
	if len(cloudConfig) == 0 {
		log.Warningf("no cloud-config data to decode")
		return []byte{}, nil
	}

	var err error
	bytes := []byte("#cloud-config\n")

	for k, v := range cloudConfig {
		var jsonData []byte
		if jsonData, err = v.MarshalJSON(); err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}

		var structData interface{}
		if err := json.Unmarshal(jsonData, &structData); err != nil {
			log.Debugf("failed to decode %s (%s): %s", k, string(jsonData), err.Error())
			return nil, fmt.Errorf("%s: %w", k, err)
		}

		var yamlData []byte
		if yamlData, err = yaml.Marshal(structData); err != nil {
			return nil, err
		}

		bytes = append(bytes, append([]byte(fmt.Sprintf("%s:\n", k)), yamlData...)...)
	}

	return bytes, nil
}

// GetSettingsValue find the given name in Rancher settings and returns its value if found
func GetSettingsValue(ctx context.Context, cli client.Client, name string) (string, error) {
	setting := &managementv3.Setting{}
	if err := cli.Get(ctx, types.NamespacedName{Name: name}, setting); err != nil {
		log.Errorf("Error getting %s setting: %s", name, err.Error())
		return "", err
	}
	return setting.Value, nil
}

// GetRancherCACert returns the cacerts included within Rancher settings. If not configured
// returns an empty string
func GetRancherCACert(ctx context.Context, cli client.Client) string {
	cacert, err := GetSettingsValue(ctx, cli, "cacerts")
	if err != nil {
		log.Errorf("Error getting cacerts: %s", err.Error())
	}

	if cacert == "" {
		if cacert, err = GetSettingsValue(ctx, cli, "internal-cacerts"); err != nil {
			log.Errorf("Error getting internal-cacerts: %s", err.Error())
			return ""
		}
	}
	return cacert
}

func IsObjectOwned(obj *metav1.ObjectMeta, uid types.UID) bool {
	for _, owner := range obj.GetOwnerReferences() {
		if owner.UID == uid {
			return true
		}
	}
	return false
}

func IsHTTP(uri string) bool {
	parsed, err := url.Parse(uri)
	if err != nil {
		return false
	}

	return strings.HasPrefix(parsed.Scheme, "http")
}

func PlanChecksum(input []byte) string {
	h := sha256.New()
	h.Write(input)

	return fmt.Sprintf("%x", h.Sum(nil))
}
