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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
)

type FakeManagedOSVersionChannel struct {
	generic.ControllerMeta
}

func (f FakeManagedOSVersionChannel) UpdateStatus(channel *v1beta1.ManagedOSVersionChannel) (*v1beta1.ManagedOSVersionChannel, error) {
	return channel, nil
}

// Not used

func (f FakeManagedOSVersionChannel) Create(channel *v1beta1.ManagedOSVersionChannel) (*v1beta1.ManagedOSVersionChannel, error) {
	panic("Create implement me")
}
func (f FakeManagedOSVersionChannel) Update(channel *v1beta1.ManagedOSVersionChannel) (*v1beta1.ManagedOSVersionChannel, error) {
	panic("Update implement me")
}
func (f FakeManagedOSVersionChannel) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	panic("Delete implement me")
}
func (f FakeManagedOSVersionChannel) Get(namespace, name string, options metav1.GetOptions) (*v1beta1.ManagedOSVersionChannel, error) {
	panic("Get implement me")
}
func (f FakeManagedOSVersionChannel) List(namespace string, opts metav1.ListOptions) (*v1beta1.ManagedOSVersionChannelList, error) {
	panic("List implement me")
}
func (f FakeManagedOSVersionChannel) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("Watch implement me")
}
func (f FakeManagedOSVersionChannel) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.ManagedOSVersionChannel, err error) {
	panic("Patch implement me")
}
func (f FakeManagedOSVersionChannel) OnChange(ctx context.Context, name string, sync elmcontrollers.ManagedOSVersionChannelHandler) {
	panic("OnChange implement me")
}
func (f FakeManagedOSVersionChannel) OnRemove(ctx context.Context, name string, sync elmcontrollers.ManagedOSVersionChannelHandler) {
	panic("OnRemove implement me")
}
func (f FakeManagedOSVersionChannel) Enqueue(namespace, name string) { panic("Enqueue implement me") }
func (f FakeManagedOSVersionChannel) EnqueueAfter(namespace, name string, duration time.Duration) {
	panic("EnqueueAfter implement me")
}
func (f FakeManagedOSVersionChannel) Cache() elmcontrollers.ManagedOSVersionChannelCache {
	panic("Cache implement me")
}
