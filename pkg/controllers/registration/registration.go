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

package registration

import (
	"context"

	elm "github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/clients"
	elmcontrollers "github.com/rancher/elemental-operator/pkg/generated/controllers/elemental.cattle.io/v1beta1"
	"github.com/rancher/wrangler/pkg/randomtoken"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
)

var controllerName = "machine-registration"

type handler struct {
	ctx       context.Context
	Recorder  record.EventRecorder
	clients   *clients.Clients
	serverURL string
}

func Register(ctx context.Context, clients *clients.Clients, serverURL string) {
	h := handler{
		ctx:       ctx,
		clients:   clients,
		serverURL: serverURL,
		Recorder:  clients.EventRecorder(controllerName),
	}
	elmcontrollers.RegisterMachineRegistrationStatusHandler(ctx, clients.Elemental.MachineRegistration(), "Ready", controllerName, h.OnChange)
	h.clients.Elemental.MachineRegistration().OnRemove(ctx, controllerName, h.OnRemove)
}

func (h *handler) OnChange(obj *elm.MachineRegistration, status elm.MachineRegistrationStatus) (elm.MachineRegistrationStatus, error) {
	var err error

	if status.RegistrationToken == "" {
		status.RegistrationToken, err = randomtoken.Generate()
		if err != nil {
			h.Recorder.Event(obj, corev1.EventTypeWarning, "error", err.Error())
			return status, err
		}
	}

	status.RegistrationURL = h.serverURL + "/elemental/registration/" + status.RegistrationToken

	_, err = h.clients.RBAC.Role().Create(&rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name,
			Namespace: obj.Namespace,
		},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Verbs:     []string{"get", "watch", "list", "update", "patch"},
			Resources: []string{"secrets"},
		}, {
			APIGroups: []string{"management.cattle.io"},
			Verbs:     []string{"get", "watch", "list"},
			Resources: []string{"settings"},
		},
		},
	})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return status, err
	}

	_, err = h.clients.Core.ServiceAccount().Create(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name,
			Namespace: obj.Namespace,
		},
	})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return status, err
	}

	_, err = h.clients.RBAC.RoleBinding().Create(&rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: obj.Namespace,
			Name:      obj.Name,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      obj.Name,
			Namespace: obj.Namespace,
		}},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     obj.Name,
			APIGroup: "rbac.authorization.k8s.io",
		},
	})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return status, err
	}

	status.ServiceAccountRef = &corev1.ObjectReference{
		Kind:      "ServiceAccount",
		Namespace: obj.Namespace,
		Name:      obj.Name,
	}

	return status, nil
}

func (h *handler) OnRemove(_ string, obj *elm.MachineRegistration) (*elm.MachineRegistration, error) {
	err := h.clients.RBAC.RoleBinding().Delete(obj.Namespace, obj.Name, &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	err = h.clients.RBAC.Role().Delete(obj.Namespace, obj.Name, &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	err = h.clients.Core.ServiceAccount().Delete(obj.Namespace, obj.Name, &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	return nil, nil
}
