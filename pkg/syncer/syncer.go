/*
Copyright © 2022 - 2024 SUSE LLC

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
	"fmt"
	"io"
	"strings"
	"time"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/object"

	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

// default paths here is where the syncer container stores serialized that
// and from where the elemental-operator display command reads it from
const (
	jsonType        = "json"
	customType      = "custom"
	defaultMountDir = "/data"
	defaultOutFile  = defaultMountDir + "/output"
	logsTimeout     = 30
)

type commons struct {
	MountPath  string `json:"mountPath"`
	OutputFile string `json:"outputFile"`
}

type CustomSyncer struct {
	upgradev1.ContainerSpec
	commons
}

type JSONSyncer struct {
	URI     string `json:"uri"`
	Timeout string `json:"timeout"`
	image   string
	commons
}

type Syncer interface {
	ToContainers() []corev1.Container
	GetMountPath() string
	GetOutputFile() string
}

type Provider interface {
	NewOSVersionsSyncer(spec elementalv1.ManagedOSVersionChannelSpec, operatorImage string) (Syncer, error)
	ReadPodLogs(ctx context.Context, kcl *kubernetes.Clientset, pod *corev1.Pod, container string) ([]byte, error)
}

type DefaultProvider struct{}

func (sp DefaultProvider) NewOSVersionsSyncer(spec elementalv1.ManagedOSVersionChannelSpec, operatorImage string) (Syncer, error) {
	common := commons{
		MountPath:  defaultMountDir,
		OutputFile: defaultOutFile,
	}
	switch strings.ToLower(spec.Type) {
	case jsonType:
		js := &JSONSyncer{commons: common, image: operatorImage}
		err := object.RenderRawExtension(spec.Options, js)
		if err != nil {
			return nil, err
		}
		return js, nil
	case customType:
		cs := &CustomSyncer{commons: common}
		err := object.RenderRawExtension(spec.Options, cs)
		if err != nil {
			return nil, err
		}
		return cs, nil
	default:
		return nil, fmt.Errorf("unknown version channel type '%s'", spec.Type)
	}
}

func (sp DefaultProvider) ReadPodLogs(ctx context.Context, kcl *kubernetes.Clientset, pod *corev1.Pod, container string) ([]byte, error) {
	logger := ctrl.LoggerFrom(ctx)
	logCtx, cancel := context.WithTimeout(context.Background(), logsTimeout*time.Second)

	defer cancel()

	var podLogs io.ReadCloser

	req := kcl.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container: container})
	podLogs, err := req.Stream(logCtx)
	if err != nil {
		logger.Error(err, "failed opening stream")
		return nil, err
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		logger.Error(err, "failed copying logs to buffer")
		return nil, err
	}

	logger.V(log.DebugDepth).Info("Got raw versions", "json", buf.String())
	return buf.Bytes(), nil
}

func (c *CustomSyncer) ToContainers() []corev1.Container {
	return []corev1.Container{
		{
			VolumeMounts: []corev1.VolumeMount{{Name: "output",
				MountPath: c.MountPath,
			}},
			Name:            "runner",
			Image:           c.Image,
			ImagePullPolicy: corev1.PullAlways,
			Command:         c.Command,
			Args:            c.Args,
			EnvFrom:         c.EnvFrom,
			Env:             c.Env,
		},
	}
}

func (c *CustomSyncer) GetMountPath() string {
	return c.MountPath
}

func (c *CustomSyncer) GetOutputFile() string {
	return c.OutputFile
}

func (j *JSONSyncer) ToContainers() []corev1.Container {
	envVars := []corev1.EnvVar{}

	if j.Timeout != "" {
		envVars = append(envVars, corev1.EnvVar{Name: elementalv1.TimeoutEnvVar, Value: j.Timeout})
	}
	return []corev1.Container{
		{
			VolumeMounts: []corev1.VolumeMount{{Name: "output",
				MountPath: j.MountPath,
			}},
			Name:    "runner",
			Image:   j.image,
			Command: []string{},
			Args:    []string{"download", "--url", j.URI, "--file", j.OutputFile},
			Env:     envVars,
		},
	}
}

func (j *JSONSyncer) GetMountPath() string {
	return j.MountPath
}

func (j *JSONSyncer) GetOutputFile() string {
	return j.OutputFile
}
