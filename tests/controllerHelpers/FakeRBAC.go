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

	rbaccontrollers "github.com/rancher/wrangler/pkg/generated/controllers/rbac/v1"
	"github.com/rancher/wrangler/pkg/generic"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"time"
)

type FakeRBAC struct {
}

func (f *FakeRBAC) ClusterRole() rbaccontrollers.ClusterRoleController {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRBAC) ClusterRoleBinding() rbaccontrollers.ClusterRoleBindingController {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRBAC) Role() rbaccontrollers.RoleController {
	return &FakeRole{}
}

func (f *FakeRBAC) RoleBinding() rbaccontrollers.RoleBindingController {
	return &FakeRoleBinding{}
}

type FakeRole struct {
}

func (f *FakeRole) Informer() cache.SharedIndexInformer {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRole) GroupVersionKind() schema.GroupVersionKind {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRole) AddGenericHandler(ctx context.Context, name string, handler generic.Handler) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRole) AddGenericRemoveHandler(ctx context.Context, name string, handler generic.Handler) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRole) Updater() generic.Updater {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRole) Create(role *rbacv1.Role) (*rbacv1.Role, error) {
	return role, nil
}

func (f *FakeRole) Update(role *rbacv1.Role) (*rbacv1.Role, error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRole) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return nil
}

func (f *FakeRole) Get(namespace, name string, options metav1.GetOptions) (*rbacv1.Role, error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRole) List(namespace string, opts metav1.ListOptions) (*rbacv1.RoleList, error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRole) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRole) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *rbacv1.Role, err error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRole) OnChange(ctx context.Context, name string, sync rbaccontrollers.RoleHandler) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRole) OnRemove(ctx context.Context, name string, sync rbaccontrollers.RoleHandler) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRole) Enqueue(namespace, name string) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRole) EnqueueAfter(namespace, name string, duration time.Duration) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRole) Cache() rbaccontrollers.RoleCache {
	//TODO implement me
	panic("implement me")
}

type FakeRoleBinding struct{}

func (f *FakeRoleBinding) Informer() cache.SharedIndexInformer {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRoleBinding) GroupVersionKind() schema.GroupVersionKind {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRoleBinding) AddGenericHandler(ctx context.Context, name string, handler generic.Handler) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRoleBinding) AddGenericRemoveHandler(ctx context.Context, name string, handler generic.Handler) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRoleBinding) Updater() generic.Updater {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRoleBinding) Create(binding *rbacv1.RoleBinding) (*rbacv1.RoleBinding, error) {
	return binding, nil
}

func (f *FakeRoleBinding) Update(binding *rbacv1.RoleBinding) (*rbacv1.RoleBinding, error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRoleBinding) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return nil
}

func (f *FakeRoleBinding) Get(namespace, name string, options metav1.GetOptions) (*rbacv1.RoleBinding, error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRoleBinding) List(namespace string, opts metav1.ListOptions) (*rbacv1.RoleBindingList, error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRoleBinding) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRoleBinding) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *rbacv1.RoleBinding, err error) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRoleBinding) OnChange(ctx context.Context, name string, sync rbaccontrollers.RoleBindingHandler) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRoleBinding) OnRemove(ctx context.Context, name string, sync rbaccontrollers.RoleBindingHandler) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRoleBinding) Enqueue(namespace, name string) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRoleBinding) EnqueueAfter(namespace, name string, duration time.Duration) {
	//TODO implement me
	panic("implement me")
}

func (f *FakeRoleBinding) Cache() rbaccontrollers.RoleBindingCache {
	//TODO implement me
	panic("implement me")
}
