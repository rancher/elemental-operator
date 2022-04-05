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

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	provv1 "github.com/rancher-sandbox/rancheros-operator/pkg/apis/rancheros.cattle.io/v1"
	"github.com/rancher-sandbox/rancheros-operator/pkg/clients"
)

type CustomSyncer struct {
	upgradev1.ContainerSpec
}

func (j *CustomSyncer) toContainers() []corev1.Container {
	return []corev1.Container{
		{
			VolumeMounts: []corev1.VolumeMount{{Name: "output",
				MountPath: "/output",
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

func (j *CustomSyncer) sync(s provv1.ManagedOSVersionChannel, c *clients.Clients) ([]provv1.ManagedOSVersion, error) {
	logrus.Infof("Syncing '%s/%s'", s.Namespace, s.Name)

	p, err := c.Core.Pod().Get(s.Namespace, s.Name, v1.GetOptions{})
	if err != nil {
		_, err = c.Core.Pod().Create(&corev1.Pod{
			TypeMeta: v1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Pod",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      s.Name,
				Namespace: s.Namespace,
				OwnerReferences: []v1.OwnerReference{
					*v1.NewControllerRef(&s, provv1.SchemeGroupVersion.WithKind("ManagedOSVersionChannel")),
				},
			},
			Spec: corev1.PodSpec{
				RestartPolicy:  corev1.RestartPolicyOnFailure,
				InitContainers: j.toContainers(),
				Volumes: []corev1.Volume{{
					Name:         "output",
					VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				}},
				Containers: []corev1.Container{{
					VolumeMounts: []corev1.VolumeMount{{Name: "output",
						MountPath: "/output",
					}},
					Name:    "pause",
					Image:   "busybox",
					Command: []string{"/bin/sh", "-c"},
					Args:    []string{"cat /output/data && sleep 9000000000"},
				},
				},
			},
		})

		// Requeueing
		return nil, err
	}

	// TODO Check container state before getting logs.
	terminated := len(p.Status.InitContainerStatuses) > 0 && p.Status.InitContainerStatuses[0].Name == "runner" &&
		p.Status.InitContainerStatuses[0].State.Terminated != nil && p.Status.InitContainerStatuses[0].State.Terminated.ExitCode == 0
	failed := len(p.Status.InitContainerStatuses) > 0 && p.Status.InitContainerStatuses[0].Name == "runner" &&
		p.Status.InitContainerStatuses[0].State.Terminated != nil && p.Status.InitContainerStatuses[0].State.Terminated.ExitCode != 0
	if !terminated {
		logrus.Infof("Waiting for '%s/%s' to finish", p.Namespace, p.Name)
		return nil, err
	} else if failed {
		// reattempt
		logrus.Infof("'%s/%s' failed, retrying", p.Namespace, p.Name)

		err = c.Core.Pod().Delete(p.Namespace, p.Name, &v1.DeleteOptions{})
		if err != nil {
			return nil, err
		}
		return nil, err
	}

	req := c.K8s.CoreV1().Pods(p.Namespace).GetLogs(p.Name, &corev1.PodLogOptions{Container: "pause"})
	podLogs, err := req.Stream(context.Background())
	if err != nil {
		return nil, fmt.Errorf("error in opening stream")
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return nil, fmt.Errorf("error in copy information from podLogs to buf")
	}

	err = c.Core.Pod().Delete(p.Namespace, p.Name, &v1.DeleteOptions{})
	if err != nil {
		return nil, err
	}

	logrus.Infof("Got '%s' from '%s/%s'", buf.String(), s.Namespace, s.Name)
	res := []provv1.ManagedOSVersion{}

	err = json.Unmarshal(buf.Bytes(), &res)
	if err != nil {
		return nil, err
	}

	return res, nil
}
