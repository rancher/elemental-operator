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

	ranchercontrollers "github.com/rancher/elemental-operator/pkg/generated/controllers/management.cattle.io/v3"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

type FakeSetting struct {
}

func (f FakeSetting) Informer() cache.SharedIndexInformer {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) GroupVersionKind() schema.GroupVersionKind {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) AddGenericHandler(ctx context.Context, name string, handler generic.Handler) {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) AddGenericRemoveHandler(ctx context.Context, name string, handler generic.Handler) {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) Updater() generic.Updater {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) Create(setting *v3.Setting) (*v3.Setting, error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) Update(setting *v3.Setting) (*v3.Setting, error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) Delete(name string, options *metav1.DeleteOptions) error {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) Get(name string, options metav1.GetOptions) (*v3.Setting, error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) List(opts metav1.ListOptions) (*v3.SettingList, error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v3.Setting, err error) {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) OnChange(ctx context.Context, name string, sync ranchercontrollers.SettingHandler) {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) OnRemove(ctx context.Context, name string, sync ranchercontrollers.SettingHandler) {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) Enqueue(name string) {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) EnqueueAfter(name string, duration time.Duration) {
	//TODO implement me
	panic("implement me")
}

func (f FakeSetting) Cache() ranchercontrollers.SettingCache {
	return &FakeSettingCache{}
}
