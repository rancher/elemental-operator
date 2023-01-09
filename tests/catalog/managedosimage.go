/*
Copyright © SUSE LLC

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

package catalog

import (
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewManagedOSImage(namespace string, name string, targets []elementalv1.BundleTarget, mosImage string, mosVersionName string) *elementalv1.ManagedOSImage {
	cordon := false

	return &elementalv1.ManagedOSImage{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "elemental.cattle.io/v1beta1",
			Kind:       "ManagedOSImage",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: elementalv1.ManagedOSImageSpec{
			OSImage:              mosImage,
			ManagedOSVersionName: mosVersionName,
			Cordon:               &cordon,
			Targets:              targets,
		},
	}
}

func DrainOSImage(namespace string, name string, managedOSVersion string, drainSpec *upgradev1.DrainSpec) *elementalv1.ManagedOSImage {
	cordon := false
	return &elementalv1.ManagedOSImage{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "elemental.cattle.io/v1beta1",
			Kind:       "ManagedOSImage",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: elementalv1.ManagedOSImageSpec{
			Cordon:               &cordon,
			ManagedOSVersionName: managedOSVersion,
			Drain:                drainSpec,
		},
	}
}
