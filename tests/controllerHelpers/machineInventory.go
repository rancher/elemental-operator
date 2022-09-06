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
	"github.com/pkg/errors"
	"github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	elmcontrollers "github.com/rancher/elemental-operator/pkg/generated/controllers/elemental.cattle.io/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
)

// FakeMachineInventoryIndexer is a fake index cache
// TODO: Fake a proper index cache? We could probably have a generic one for all controllers testing
type FakeMachineInventoryIndexer struct {
	inventory []*v1beta1.MachineInventory
}

func (f *FakeMachineInventoryIndexer) AddToIndex(inv *v1beta1.MachineInventory) {
	f.inventory = append(f.inventory, inv)
}

func (f *FakeMachineInventoryIndexer) CleanIndex() {
	f.inventory = []*v1beta1.MachineInventory{}
}

func (f *FakeMachineInventoryIndexer) Get(namespace, name string) (*v1beta1.MachineInventory, error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeMachineInventoryIndexer) GetByIndex(indexName, key string) ([]*v1beta1.MachineInventory, error) {
	return f.inventory, nil
}

func (f *FakeMachineInventoryIndexer) List(namespace string, selector labels.Selector) ([]*v1beta1.MachineInventory, error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeMachineInventoryIndexer) AddIndexer(indexName string, indexer elmcontrollers.MachineInventoryIndexer) {
	//TODO implement me
	panic("implement me")
}

// FakeMachineInventory is the fake inventory controller
type FakeMachineInventory struct {
	inventory          *v1beta1.MachineInventory
	UpdateStatusCalled bool
	FailOnUpdateStatus bool
}

func NewFakeInventory(inv *v1beta1.MachineInventory) *FakeMachineInventory {
	return &FakeMachineInventory{inventory: inv}
}

func (f *FakeMachineInventory) GetInventory() *v1beta1.MachineInventory {
	return f.inventory
}

func (f *FakeMachineInventory) Create(inventory *v1beta1.MachineInventory) (*v1beta1.MachineInventory, error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeMachineInventory) Update(inventory *v1beta1.MachineInventory) (*v1beta1.MachineInventory, error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeMachineInventory) UpdateStatus(inventory *v1beta1.MachineInventory) (*v1beta1.MachineInventory, error) {
	if f.FailOnUpdateStatus {
		return nil, errors.New("test")
	}
	f.inventory = inventory
	f.UpdateStatusCalled = true
	return f.inventory, nil
}

func (f *FakeMachineInventory) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	//TODO implement me
	panic("implement me")
}

func (f *FakeMachineInventory) Get(namespace, name string, options metav1.GetOptions) (*v1beta1.MachineInventory, error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeMachineInventory) List(namespace string, opts metav1.ListOptions) (*v1beta1.MachineInventoryList, error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeMachineInventory) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeMachineInventory) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.MachineInventory, err error) {
	//TODO implement me
	panic("implement me")
}
