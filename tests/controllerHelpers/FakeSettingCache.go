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
	v3generated "github.com/rancher/elemental-operator/pkg/generated/controllers/management.cattle.io/v3"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"k8s.io/apimachinery/pkg/labels"
)

type FakeSettingCache struct {
	Index map[string]*v3.Setting
}

// Interface methods

func (f *FakeSettingCache) Get(name string) (*v3.Setting, error) {
	if f.Index != nil {
		setting := f.Index[name]
		if setting == nil {
			return nil, errors.New("not found")
		}
		return setting, nil
	}

	return nil, errors.New("not found")
}
func (f *FakeSettingCache) List(selector labels.Selector) ([]*v3.Setting, error) {
	panic("List not implemented")
}
func (f *FakeSettingCache) AddIndexer(indexName string, indexer v3generated.SettingIndexer) {
	panic("List not implemented")
}
func (f *FakeSettingCache) GetByIndex(indexName, key string) ([]*v3.Setting, error) {
	panic("List not implemented")
}

// Extra methods for testing

func (f *FakeSettingCache) AddToIndex(name string, setting *v3.Setting) {
	if f.Index == nil {
		f.Index = map[string]*v3.Setting{}
	}
	f.Index[name] = setting
}
