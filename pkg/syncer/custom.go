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

package syncer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
)

type CustomSyncer struct {
	upgradev1.ContainerSpec
	MountPath  string `json:"mountPath"`
	OutputFile string `json:"outputFile"`

	kcl           *kubernetes.Clientset
	operatorImage string
}

const requeue = 5 * time.Second

func (j *CustomSyncer) toContainers(mount string) []corev1.Container {
	return []corev1.Container{
		{
			VolumeMounts: []corev1.VolumeMount{{Name: "output",
				MountPath: mount,
			}},
			Name:    "runner",
			Image:   j.Image,
			Command: j.Command,
			Args:    j.Args,
			EnvFrom: j.EnvFrom,
			Env:     j.Env,
		},
	}
}

func (j *CustomSyncer) Sync(ctx context.Context, cl client.Client, ch *elementalv1.ManagedOSVersionChannel) ([]elementalv1.ManagedOSVersion, bool, error) {
	logger := ctrl.LoggerFrom(ctx)
	logger.Info("Syncing (Custom)", "ManagedOSVersionChannel", ch.Name)

	mountDir := j.MountPath
	outFile := j.OutputFile
	if mountDir == "" {
		mountDir = "/data"
	}
	if outFile == "" {
		outFile = "/data/output"
	}

	serviceAccount := false
	pod := &corev1.Pod{}
	err := cl.Get(ctx, client.ObjectKey{
		Namespace: ch.Namespace,
		Name:      ch.Name,
	}, pod)
	if err != nil {
		logger.V(5).Info("Failed to get", "pod", ch.Name, "error", err)
		err = cl.Create(ctx, &corev1.Pod{
			TypeMeta: v1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Pod",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      ch.Name,
				Namespace: ch.Namespace,
				OwnerReferences: []v1.OwnerReference{
					{
						APIVersion: elementalv1.GroupVersion.String(),
						Kind:       "ManagedOSVersionChannel",
						Name:       ch.Name,
						UID:        ch.UID,
						Controller: pointer.Bool(true),
					},
				},
			},
			Spec: corev1.PodSpec{
				RestartPolicy:                corev1.RestartPolicyOnFailure,
				AutomountServiceAccountToken: &serviceAccount,
				InitContainers:               j.toContainers(mountDir),
				Volumes: []corev1.Volume{{
					Name:         "output",
					VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				}},
				Containers: []corev1.Container{{
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "output",
						MountPath: mountDir,
					}},
					Name:    "pause",
					Image:   j.operatorImage,
					Command: []string{},
					Args:    []string{"display", "--file", outFile},
				},
				},
			},
		})

		return nil, true, err
	}

	terminated := len(pod.Status.InitContainerStatuses) > 0 && pod.Status.InitContainerStatuses[0].Name == "runner" &&
		pod.Status.InitContainerStatuses[0].State.Terminated != nil

	failed := terminated && pod.Status.InitContainerStatuses[0].State.Terminated.ExitCode != 0

	if !terminated {
		logger.Info("Waiting pod to finish", "pod", pod.Name)

		return nil, true, nil
	} else if failed {
		logger.Info("Synchronization pod failed", "pod", pod.Name)

		err := cl.Delete(ctx, pod)
		return nil, true, err
	}

	logCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := j.kcl.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container: "pause"})
	podLogs, err := req.Stream(logCtx)
	if err != nil {
		return nil, true, fmt.Errorf("error in opening stream")
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return nil, true, fmt.Errorf("error in copy information from podLogs to buf")
	}

	err = cl.Delete(ctx, pod)
	if err != nil {
		return nil, true, err
	}

	logger.Info("Got raw versions", "json", buf.String())
	res := []elementalv1.ManagedOSVersion{}

	err = json.Unmarshal(buf.Bytes(), &res)
	if err != nil {
		return nil, true, err
	}

	return res, false, nil
}
