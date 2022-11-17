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

package managedos

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	elm "github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/clients"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/name"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func cloudConfig(mos *elm.ManagedOSImage) ([]byte, error) {
	if mos.Spec.CloudConfig == nil || len(mos.Spec.CloudConfig.Data) == 0 {
		return []byte{}, nil
	}
	data, err := yaml.Marshal(mos.Spec.CloudConfig.Data)
	if err != nil {
		return nil, err
	}
	return append([]byte("#cloud-config\n"), data...), nil
}

func getImageVersion(mos *elm.ManagedOSImage, mv *elm.ManagedOSVersion) ([]string, string, error) {
	baseImage := mos.Spec.OSImage
	if baseImage == "" && mv != nil {
		osMeta, err := mv.Metadata()
		if err != nil {
			return []string{}, "", err
		}
		baseImage = osMeta.ImageURI
	}

	image := strings.SplitN(baseImage, ":", 2)
	version := "latest"
	if len(image) == 2 {
		version = image[1]
	}

	return image, version, nil
}

func metadataEnv(m map[string]interface{}) []corev1.EnvVar {
	// Encode metadata as environment in a slice of envVar
	envs := []corev1.EnvVar{}
	for k, v := range m {
		toEncode := ""
		switch value := v.(type) {
		case string:
			toEncode = value
		default:
			j, _ := json.Marshal(v)
			toEncode = string(j)
		}
		envs = append(envs, corev1.EnvVar{Name: strings.ToUpper(fmt.Sprintf("METADATA_%s", k)), Value: toEncode})
	}
	return envs
}

func (h *handler) objects(mos *elm.ManagedOSImage) ([]runtime.Object, error) {
	cloudConfig, err := cloudConfig(mos)
	if err != nil {
		return nil, err
	}

	concurrency := int64(1)
	if mos.Spec.Concurrency != nil {
		concurrency = *mos.Spec.Concurrency
	}

	cordon := true
	if mos.Spec.Cordon != nil {
		cordon = *mos.Spec.Cordon
	}

	var mv *elm.ManagedOSVersion

	m := &fleet.GenericMap{Data: make(map[string]interface{})}

	// if a managedOS version was specified, we fetch it for later use and store the metadata
	if mos.Spec.ManagedOSVersionName != "" {
		// ns, name
		mv, err = h.managedVersionCache.Get(mos.ObjectMeta.Namespace, mos.Spec.ManagedOSVersionName)
		if err != nil {
			h.Recorder.Event(mos, corev1.EventTypeWarning, "error", err.Error())
			return []runtime.Object{}, err
		}
		m = mv.Spec.Metadata
	}

	// XXX Issues currently standing:
	// - minVersion is not respected:
	//	 gate minVersion that are not passing validation checks with the version reported
	// - Monitoring upgrade status from the fleet bundles (reconcile to update the status to report what is the current version )
	// - Enforce a ManagedOSImage "version" that is applied to a one node only. Or check out if either fleet is already doing that
	image, version, err := getImageVersion(mos, mv)
	if err != nil {
		h.Recorder.Event(mos, corev1.EventTypeWarning, "error", err.Error())
		return []runtime.Object{}, err
	}

	selector := mos.Spec.NodeSelector
	if selector == nil {
		selector = &metav1.LabelSelector{}
	}

	upgradeContainerSpec := mos.Spec.UpgradeContainer
	if mv != nil && upgradeContainerSpec == nil {
		upgradeContainerSpec = mv.Spec.UpgradeContainer
	}

	if upgradeContainerSpec == nil {
		upgradeContainerSpec = &upgradev1.ContainerSpec{
			Image: PrefixPrivateRegistry(image[0], h.DefaultRegistry),
			Command: []string{
				"/usr/sbin/suc-upgrade",
			},
		}
	}

	// Encode metadata from the spec as environment in the upgrade spec pod
	metadataEnv := metadataEnv(m.Data)

	// metadata envs overwrites any other specified
	keys := map[string]interface{}{}
	for _, e := range metadataEnv {
		keys[e.Name] = nil
	}

	for _, e := range upgradeContainerSpec.Env {
		if _, ok := keys[e.Name]; !ok {
			metadataEnv = append(metadataEnv, e)
		}
	}

	sort.Slice(metadataEnv, func(i, j int) bool {
		dat := []string{metadataEnv[i].Name, metadataEnv[j].Name}
		sort.Strings(dat)
		return dat[0] == metadataEnv[i].Name
	})

	upgradeContainerSpec.Env = metadataEnv

	objectName := name.SafeConcatName("os-upgrader", mos.Name, string(mos.UID))

	return []runtime.Object{
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: objectName,
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"update", "get", "list", "watch", "patch"},
					APIGroups: []string{""},
					Resources: []string{"nodes"},
				},
				{
					Verbs:     []string{"list"},
					APIGroups: []string{""},
					Resources: []string{"pods"},
				},
			},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: objectName,
			},
			Subjects: []rbacv1.Subject{{
				Kind:      "ServiceAccount",
				Name:      objectName,
				Namespace: clients.SystemNamespace,
			}},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     objectName,
			},
		},
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      objectName,
				Namespace: clients.SystemNamespace,
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      objectName,
				Namespace: clients.SystemNamespace,
				Labels: map[string]string{
					v1beta1.ManagedSecretLabel: "true",
				},
			},
			Data: map[string][]byte{
				"cloud-config": cloudConfig,
			},
		},
		&upgradev1.Plan{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Plan",
				APIVersion: "upgrade.cattle.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      objectName,
				Namespace: clients.SystemNamespace,
			},
			Spec: upgradev1.PlanSpec{
				Concurrency: concurrency,
				Version:     version,
				Tolerations: []corev1.Toleration{{
					Operator: corev1.TolerationOpExists,
				}},
				ServiceAccountName: objectName,
				NodeSelector:       selector,
				Cordon:             cordon,
				Drain:              mos.Spec.Drain,
				Prepare:            mos.Spec.Prepare,
				Secrets: []upgradev1.SecretSpec{{
					Name: objectName,
					Path: "/run/data",
				}},
				Upgrade: upgradeContainerSpec,
			},
		},
	}, nil
}

func PrefixPrivateRegistry(image, prefix string) string {
	if prefix == "" {
		return image
	}
	return prefix + "/" + image
}
