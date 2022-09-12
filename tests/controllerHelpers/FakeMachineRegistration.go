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
	"time"

	"github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	elmcontrollers "github.com/rancher/elemental-operator/pkg/generated/controllers/elemental.cattle.io/v1beta1"
	"github.com/rancher/wrangler/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

type FakeMachineRegistration struct {
}

func (f FakeMachineRegistration) Informer() cache.SharedIndexInformer {
	//TODO implement me
	panic("implement me")
}

func (f FakeMachineRegistration) GroupVersionKind() schema.GroupVersionKind {
	//TODO implement me
	panic("implement me")
}

func (f FakeMachineRegistration) AddGenericHandler(ctx context.Context, name string, handler generic.Handler) {
}

func (f FakeMachineRegistration) AddGenericRemoveHandler(ctx context.Context, name string, handler generic.Handler) {
	//TODO implement me
	panic("implement me")
}

func (f FakeMachineRegistration) Updater() generic.Updater {
	//TODO implement me
	panic("implement me")
}

func (f FakeMachineRegistration) Create(registration *v1beta1.MachineRegistration) (*v1beta1.MachineRegistration, error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeMachineRegistration) Update(registration *v1beta1.MachineRegistration) (*v1beta1.MachineRegistration, error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeMachineRegistration) UpdateStatus(registration *v1beta1.MachineRegistration) (*v1beta1.MachineRegistration, error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeMachineRegistration) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	//TODO implement me
	panic("implement me")
}

func (f FakeMachineRegistration) Get(namespace, name string, options metav1.GetOptions) (*v1beta1.MachineRegistration, error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeMachineRegistration) List(namespace string, opts metav1.ListOptions) (*v1beta1.MachineRegistrationList, error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeMachineRegistration) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeMachineRegistration) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.MachineRegistration, err error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeMachineRegistration) OnChange(ctx context.Context, name string, sync elmcontrollers.MachineRegistrationHandler) {
	//TODO implement me
	panic("implement me")
}

func (f FakeMachineRegistration) OnRemove(ctx context.Context, name string, sync elmcontrollers.MachineRegistrationHandler) {
}

func (f FakeMachineRegistration) Enqueue(namespace, name string) {
	//TODO implement me
	panic("implement me")
}

func (f FakeMachineRegistration) EnqueueAfter(namespace, name string, duration time.Duration) {
	//TODO implement me
	panic("implement me")
}

func (f FakeMachineRegistration) Cache() elmcontrollers.MachineRegistrationCache {
	//TODO implement me
	panic("implement me")
}
