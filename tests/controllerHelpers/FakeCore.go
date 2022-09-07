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

package controllerHelpers

import (
	"context"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/generic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"time"
)

type FakeCore struct {
}

func (f FakeCore) ConfigMap() corecontrollers.ConfigMapController {
	//TODO implement me
	panic("implement me")
}

func (f FakeCore) Endpoints() corecontrollers.EndpointsController {
	//TODO implement me
	panic("implement me")
}

func (f FakeCore) Event() corecontrollers.EventController {
	//TODO implement me
	panic("implement me")
}

func (f FakeCore) Namespace() corecontrollers.NamespaceController {
	//TODO implement me
	panic("implement me")
}

func (f FakeCore) Node() corecontrollers.NodeController {
	//TODO implement me
	panic("implement me")
}

func (f FakeCore) PersistentVolumeClaim() corecontrollers.PersistentVolumeClaimController {
	//TODO implement me
	panic("implement me")
}

func (f FakeCore) Pod() corecontrollers.PodController {
	//TODO implement me
	panic("implement me")
}

func (f FakeCore) Secret() corecontrollers.SecretController {
	//TODO implement me
	panic("implement me")
}

func (f FakeCore) Service() corecontrollers.ServiceController {
	//TODO implement me
	panic("implement me")
}

func (f FakeCore) ServiceAccount() corecontrollers.ServiceAccountController {
	return FakeServiceAccount{}
}

type FakeServiceAccount struct {
}

func (f FakeServiceAccount) Informer() cache.SharedIndexInformer {
	//TODO implement me
	panic("implement me")
}

func (f FakeServiceAccount) GroupVersionKind() schema.GroupVersionKind {
	//TODO implement me
	panic("implement me")
}

func (f FakeServiceAccount) AddGenericHandler(ctx context.Context, name string, handler generic.Handler) {
	//TODO implement me
	panic("implement me")
}

func (f FakeServiceAccount) AddGenericRemoveHandler(ctx context.Context, name string, handler generic.Handler) {
	//TODO implement me
	panic("implement me")
}

func (f FakeServiceAccount) Updater() generic.Updater {
	//TODO implement me
	panic("implement me")
}

func (f FakeServiceAccount) Create(account *corev1.ServiceAccount) (*corev1.ServiceAccount, error) {
	return account, nil
}

func (f FakeServiceAccount) Update(account *corev1.ServiceAccount) (*corev1.ServiceAccount, error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeServiceAccount) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return nil
}

func (f FakeServiceAccount) Get(namespace, name string, options metav1.GetOptions) (*corev1.ServiceAccount, error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeServiceAccount) List(namespace string, opts metav1.ListOptions) (*corev1.ServiceAccountList, error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeServiceAccount) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeServiceAccount) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *corev1.ServiceAccount, err error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeServiceAccount) OnChange(ctx context.Context, name string, sync corecontrollers.ServiceAccountHandler) {
	//TODO implement me
	panic("implement me")
}

func (f FakeServiceAccount) OnRemove(ctx context.Context, name string, sync corecontrollers.ServiceAccountHandler) {
	//TODO implement me
	panic("implement me")
}

func (f FakeServiceAccount) Enqueue(namespace, name string) {
	//TODO implement me
	panic("implement me")
}

func (f FakeServiceAccount) EnqueueAfter(namespace, name string, duration time.Duration) {
	//TODO implement me
	panic("implement me")
}

func (f FakeServiceAccount) Cache() corecontrollers.ServiceAccountCache {
	//TODO implement me
	panic("implement me")
}
