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
	"context"
	"fmt"

	"github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	elm "github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/clients"
	elmcontrollers "github.com/rancher/elemental-operator/pkg/generated/controllers/elemental.cattle.io/v1beta1"
	fleetcontrollers "github.com/rancher/elemental-operator/pkg/generated/controllers/fleet.cattle.io/v1alpha1"
	"github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	"github.com/rancher/wrangler/pkg/name"
	"github.com/rancher/wrangler/pkg/relatedresource"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

var controllerName = "mos-bundle"

func Register(ctx context.Context, clients clients.ClientInterface, defaultRegistry string) {
	h := &handler{
		bundleCache:         clients.Fleet().Bundle().Cache(),
		managedVersionCache: clients.Elemental().ManagedOSVersion().Cache(),
		Recorder:            clients.EventRecorder(controllerName),
		DefaultRegistry:     defaultRegistry,
	}

	relatedresource.Watch(ctx,
		"mcc-from-bundle-trigger",
		relatedresource.OwnerResolver(true, elm.SchemeGroupVersion.String(), "ManagedOSImage"),
		clients.Elemental().ManagedOSImage(),
		clients.Fleet().Bundle())
	elmcontrollers.RegisterManagedOSImageGeneratingHandler(ctx,
		clients.Elemental().ManagedOSImage(),
		clients.Apply().
			WithSetOwnerReference(true, true).
			WithCacheTypes(
				clients.Elemental().ManagedOSImage(),
				clients.Fleet().Bundle()),
		"",
		controllerName,
		h.OnChange,
		nil)
}

type handler struct {
	bundleCache         fleetcontrollers.BundleCache
	managedVersionCache elmcontrollers.ManagedOSVersionCache
	Recorder            record.EventRecorder
	DefaultRegistry     string
}

func (h *handler) OnChange(mos *elm.ManagedOSImage, status elm.ManagedOSImageStatus) ([]runtime.Object, elm.ManagedOSImageStatus, error) {
	if mos.Spec.OSImage == "" && mos.Spec.ManagedOSVersionName == "" {
		return nil, status, nil
	}

	objs, err := h.objects(mos)
	if err != nil {
		return nil, status, err
	}

	resources, err := ToResources(objs)
	if err != nil {
		return nil, status, err
	}

	if mos.Namespace == "fleet-local" && len(mos.Spec.Targets) > 0 {
		msg := "spec.targets should be empty if in the fleet-local namespace"
		h.Recorder.Event(mos, corev1.EventTypeWarning, "error", msg)
		return nil, status, fmt.Errorf(msg)
	}

	bundle := &v1alpha1.Bundle{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.SafeConcatName("mos", mos.Name, string(mos.UID)),
			Namespace: mos.Namespace,
		},
		Spec: v1alpha1.BundleSpec{
			Resources:               resources,
			BundleDeploymentOptions: v1alpha1.BundleDeploymentOptions{DefaultNamespace: mos.Namespace},
			RolloutStrategy:         mos.Spec.ClusterRolloutStrategy,
			Targets:                 mos.Spec.Targets,
		},
	}

	if mos.Namespace == "fleet-local" {
		bundle.Spec.Targets = []v1alpha1.BundleTarget{{ClusterName: "local"}}
	}

	v1beta1.DefinedCondition.SetError(&status, v1beta1.MOSDefinedReason, nil)

	status, err = h.updateStatus(status, bundle)
	return []runtime.Object{
		bundle,
	}, status, err
}

func (h *handler) updateStatus(status elm.ManagedOSImageStatus, bundle *v1alpha1.Bundle) (elm.ManagedOSImageStatus, error) {
	bundle, err := h.bundleCache.Get(bundle.Namespace, bundle.Name)
	if apierrors.IsNotFound(err) {
		return status, nil
	} else if err != nil {
		return status, err
	}

	status.BundleStatus = bundle.Status
	return status, nil
}
