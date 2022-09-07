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
	elmcontrollers "github.com/rancher/elemental-operator/pkg/generated/controllers/elemental.cattle.io/v1beta1"
)

// FakeElementalV1beta1 is a struct that implements the ElementalController interface
type FakeElementalV1beta1 struct{}

func (f *FakeElementalV1beta1) MachineInventory() elmcontrollers.MachineInventoryController {
	//TODO implement me
	panic("MachineInventory implement me")
}

func (f *FakeElementalV1beta1) MachineInventorySelector() elmcontrollers.MachineInventorySelectorController {
	//TODO implement me
	panic("MachineInventorySelector implement me")
}

func (f *FakeElementalV1beta1) MachineInventorySelectorTemplate() elmcontrollers.MachineInventorySelectorTemplateController {
	//TODO implement me
	panic("MachineInventorySelectorTemplate implement me")
}

func (f *FakeElementalV1beta1) MachineRegistration() elmcontrollers.MachineRegistrationController {
	//TODO implement me
	panic("MachineRegistration implement me")
}

func (f *FakeElementalV1beta1) ManagedOSImage() elmcontrollers.ManagedOSImageController {
	//TODO implement me
	panic("ManagedOSImage implement me")
}

func (f *FakeElementalV1beta1) ManagedOSVersion() elmcontrollers.ManagedOSVersionController {
	//TODO implement me
	panic("ManagedOSVersion implement me")
}

func (f *FakeElementalV1beta1) ManagedOSVersionChannel() elmcontrollers.ManagedOSVersionChannelController {
	return FakeManagedOSVersionChannel{}
}
