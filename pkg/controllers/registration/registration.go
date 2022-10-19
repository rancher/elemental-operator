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
	"fmt"

	elm "github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/clients"
	elmcontrollers "github.com/rancher/elemental-operator/pkg/generated/controllers/elemental.cattle.io/v1beta1"
	ranchercontrollers "github.com/rancher/elemental-operator/pkg/generated/controllers/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/randomtoken"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
)

var controllerName = "machine-registration"

type handler struct {
	ctx          context.Context
	Recorder     record.EventRecorder
	clients      clients.ClientInterface
	settingCache ranchercontrollers.SettingCache
}

func Register(ctx context.Context, clients clients.ClientInterface) {
	h := handler{
		ctx:          ctx,
		clients:      clients,
		Recorder:     clients.EventRecorder(controllerName),
		settingCache: clients.Rancher().Setting().Cache(),
	}
	elmcontrollers.RegisterMachineRegistrationStatusHandler(ctx, clients.Elemental().MachineRegistration(), "", controllerName, h.OnChange)
	h.clients.Elemental().MachineRegistration().OnRemove(ctx, controllerName, h.OnRemove)
}

func (h *handler) OnChange(obj *elm.MachineRegistration, status elm.MachineRegistrationStatus) (elm.MachineRegistrationStatus, error) {
	var err error
	var isNewRegistration bool

	logrus.Debugf("MachineRegistration %s/%s: OnChange event", obj.Namespace, obj.Name)
	serverURL, err := h.getRancherServerURL()
	if err != nil {
		return status, err
	}

	if status.RegistrationToken == "" {
		isNewRegistration = true
		status.RegistrationToken, err = randomtoken.Generate()
		if err != nil {
			h.Recorder.Event(obj, corev1.EventTypeWarning, "error", err.Error())
			return status, err
		}
	}

	status.RegistrationURL = fmt.Sprintf("%s/elemental/registration/%s", serverURL, status.RegistrationToken)

	_, err = h.clients.RBAC().Role().Create(&rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name,
			Namespace: obj.Namespace,
			Labels: map[string]string{
				elm.ManagedSecretLabel: "true",
			},
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
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return status, err
		}
	} else {
		logrus.Debugf("Created Role %s/%s", obj.Namespace, obj.Name)
	}

	err = h.createServiceAccountWithSecret(obj.Namespace, obj.Name)
	if err != nil {
		return status, err
	}

	_, err = h.clients.RBAC().RoleBinding().Create(&rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name,
			Namespace: obj.Namespace,
			Labels: map[string]string{
				elm.ManagedSecretLabel: "true",
			},
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
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return status, err
		}
	} else {
		logrus.Debugf("Created RoleBinding %s/%s", obj.Namespace, obj.Name)
	}

	status.ServiceAccountRef = &corev1.ObjectReference{
		Kind:      "ServiceAccount",
		Namespace: obj.Namespace,
		Name:      obj.Name,
	}

	if isNewRegistration {
		logrus.Infof("Got new MachineRegistration '%s/%s': generated token '%s'", obj.Namespace, obj.Name, status.RegistrationToken)
	}

	elm.ReadyCondition.SetError(&status, elm.MachineRegistrationReadyReason, nil)

	return status, nil
}

func (h *handler) OnRemove(_ string, obj *elm.MachineRegistration) (*elm.MachineRegistration, error) {
	logrus.Infof("MachineRegistration '%s/%s' deletion event", obj.Namespace, obj.Name)
	err := h.clients.RBAC().RoleBinding().Delete(obj.Namespace, obj.Name, &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	err = h.clients.RBAC().Role().Delete(obj.Namespace, obj.Name, &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	err = h.clients.Core().ServiceAccount().Delete(obj.Namespace, obj.Name, &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	return nil, nil
}

func (h *handler) getRancherServerURL() (string, error) {
	setting, err := h.settingCache.Get("server-url")
	if err != nil {
		logrus.Errorf("Error getting server-url setting: %s", err.Error())
		return "", err
	}
	if setting.Value == "" {
		logrus.Error("server-url is not set")
		return "", fmt.Errorf("server-url is not set")
	}
	return setting.Value, nil
}

func (h *handler) createServiceAccountWithSecret(namespace string, name string) error {
	secretName := name + "-token"

	_, err := h.clients.Core().ServiceAccount().Create(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				elm.ManagedSecretLabel: "true",
			},
		},
		Secrets: []corev1.ObjectReference{
			{
				Name: secretName,
			},
		},
	})
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		// Ensure the ServiceAccount is linked to a Secret
		sa, err := h.clients.Core().ServiceAccount().Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			logrus.Warnf("Skip checks on '%s' ServiceAccount: %s", name, err.Error())
		} else {
			if len(sa.Secrets) == 0 {
				sa.Secrets = []corev1.ObjectReference{
					{
						Name: secretName,
					},
				}
				_, err = h.clients.Core().ServiceAccount().Update(sa)
				if err != nil {
					return fmt.Errorf("update %s ServiceAccount: %s", name, err.Error())
				}
				logrus.Infof("Updated ServiceAccount %s/%s Secret link", namespace, name)
			}
		}
	} else {
		logrus.Debugf("Created ServiceAccount %s/%s", namespace, name)
	}

	_, err = h.clients.Core().Secret().Create(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				elm.ManagedSecretLabel: "true",
			},
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": name,
			},
		},
		Type: "kubernetes.io/service-account-token",
	})
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("add Secret to %s ServiceAccount: %w", name, err)
		}
	} else {
		logrus.Debugf("Created Secret %s/%s", namespace, name)
	}
	return nil
}
