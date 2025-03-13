/*
Copyright © 2022 - 2025 SUSE LLC

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

package controllerhelpers

import (
	"context"
	"fmt"
	"sync"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/syncer"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

type FakeSyncer struct{}

type FakeSyncerProvider struct {
	UnknownType string
	LogsError   bool
	mu          sync.Mutex
	json        string
}

func (fs *FakeSyncer) GetMountPath() string {
	return "/fake/data"
}

func (fs *FakeSyncer) GetOutputFile() string {
	return "/fake/data/file"
}

func (fs *FakeSyncer) ToContainers() []corev1.Container {
	return []corev1.Container{
		{
			VolumeMounts: []corev1.VolumeMount{{Name: "output",
				MountPath: "/fake/data",
			}},
			Name:    "runner",
			Image:   "non/existing/image:latest",
			Command: []string{"fakecmd"},
			Args:    []string{"--file", "/fake/data/file"},
		},
	}
}

func (sp *FakeSyncerProvider) NewOSVersionsSyncer(spec elementalv1.ManagedOSVersionChannelSpec, _ string) (syncer.Syncer, error) {
	if spec.Type == sp.UnknownType {
		return &FakeSyncer{}, fmt.Errorf("Unknown type of channel")
	}
	return &FakeSyncer{}, nil
}

func (sp *FakeSyncerProvider) ReadPodLogs(_ context.Context, _ *kubernetes.Clientset, _ *corev1.Pod, _ string) ([]byte, error) {
	if sp.LogsError {
		return nil, fmt.Errorf("Failed retrieving pod logs")
	}
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return []byte(sp.json), nil
}

func (sp *FakeSyncerProvider) SetJSON(json string) {
	sp.mu.Lock()
	sp.json = json
	sp.mu.Unlock()
}
