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

package types

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	elm "github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/services/syncer/config"
)

type CustomSyncer struct {
	upgradev1.ContainerSpec

	MountPath  string `json:"mountPath"`
	OutputFile string `json:"outputFile"`
}

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

func (j *CustomSyncer) Sync(c config.Config, s elm.ManagedOSVersionChannel) ([]elm.ManagedOSVersion, error) {
	logrus.Infof("Syncing '%s/%s'", s.Namespace, s.Name)

	mountDir := j.MountPath
	outFile := j.OutputFile
	if mountDir == "" {
		mountDir = "/data"
	}
	if outFile == "" {
		outFile = "/data/output"
	}

	serviceAccount := false
	p, err := c.Clients.Core().Pod().Get(s.Namespace, s.Name, v1.GetOptions{})
	if err != nil {
		_, err = c.Clients.Core().Pod().Create(&corev1.Pod{
			TypeMeta: v1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Pod",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      s.Name,
				Namespace: s.Namespace,
				OwnerReferences: []v1.OwnerReference{
					*v1.NewControllerRef(&s, elm.SchemeGroupVersion.WithKind("ManagedOSVersionChannel")),
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
					Image:   c.OperatorImage,
					Command: []string{},
					Args:    []string{"display", "--file", outFile},
				},
				},
			},
		})

		// Requeueing
		c.Requeuer.Requeue()
		return nil, err
	}

	terminated := len(p.Status.InitContainerStatuses) > 0 && p.Status.InitContainerStatuses[0].Name == "runner" &&
		p.Status.InitContainerStatuses[0].State.Terminated != nil

	failed := terminated && p.Status.InitContainerStatuses[0].State.Terminated.ExitCode != 0

	if !terminated {
		logrus.Infof("Waiting for '%s/%s' to finish", p.Namespace, p.Name)

		c.Requeuer.Requeue()
		return nil, err
	} else if failed {
		// reattempt
		logrus.Infof("'%s/%s' failed, retrying", p.Namespace, p.Name)

		err = c.Clients.Core().Pod().Delete(p.Namespace, p.Name, &v1.DeleteOptions{})
		if err != nil {
			return nil, err
		}
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := c.Clients.K8s().CoreV1().Pods(p.Namespace).GetLogs(p.Name, &corev1.PodLogOptions{Container: "pause"})
	podLogs, err := req.Stream(ctx)
	if err != nil {
		c.Requeuer.Requeue()
		return nil, fmt.Errorf("error in opening stream")
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		c.Requeuer.Requeue()
		return nil, fmt.Errorf("error in copy information from podLogs to buf")
	}

	err = c.Clients.Core().Pod().Delete(p.Namespace, p.Name, &v1.DeleteOptions{})
	if err != nil {
		c.Requeuer.Requeue()
		return nil, err
	}

	logrus.Infof("Got '%s' from '%s/%s'", buf.String(), s.Namespace, s.Name)
	res := []elm.ManagedOSVersion{}

	err = json.Unmarshal(buf.Bytes(), &res)
	if err != nil {
		return nil, err
	}

	return res, nil
}
