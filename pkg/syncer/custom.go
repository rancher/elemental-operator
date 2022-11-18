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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
)

const defaultMountDir = "/data"
const defaultOutFile = defaultMountDir + "/output"

type CustomSyncer struct {
	upgradev1.ContainerSpec
	MountPath  string `json:"mountPath"`
	OutputFile string `json:"outputFile"`

	kcl           *kubernetes.Clientset
	operatorImage string
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

// Sync attemps to get a list of managed os versions based on the managed os version channel configuration, on success it updates the ready condition
func (j *CustomSyncer) Sync(ctx context.Context, cl client.Client, ch *elementalv1.ManagedOSVersionChannel) ([]elementalv1.ManagedOSVersion, error) {
	logger := ctrl.LoggerFrom(ctx)
	logger.Info("Syncing (Custom)", "ManagedOSVersionChannel", ch.Name)

	readyCondition := meta.FindStatusCondition(ch.Status.Conditions, elementalv1.ReadyCondition)

	// Check if synchronization had started before
	if readyCondition != nil && readyCondition.Reason == elementalv1.SyncingReason {
		return j.fecthDataFromPod(ctx, cl, ch)
	}
	// Start syncing process
	err := j.createSyncerPod(ctx, cl, ch)
	return nil, err
}

func (j *CustomSyncer) fecthDataFromPod(
	ctx context.Context, cl client.Client,
	ch *elementalv1.ManagedOSVersionChannel) ([]elementalv1.ManagedOSVersion, error) {
	logger := ctrl.LoggerFrom(ctx)

	pod := &corev1.Pod{}
	err := cl.Get(ctx, client.ObjectKey{
		Namespace: ch.Namespace,
		Name:      ch.Name,
	}, pod)
	if err != nil {
		logger.Error(err, "failed getting pod resource", "pod", pod.Name)
		return nil, err
	}

	terminated := len(pod.Status.InitContainerStatuses) > 0 && pod.Status.InitContainerStatuses[0].Name == "runner" &&
		pod.Status.InitContainerStatuses[0].State.Terminated != nil

	failed := terminated && pod.Status.InitContainerStatuses[0].State.Terminated.ExitCode != 0

	if !terminated {
		logger.Info("Waiting pod to finish, not terminated", "pod", pod.Name)
		return nil, nil
	} else if failed {
		errMsg := "Synchronization pod failed"
		logger.Error(fmt.Errorf(errMsg), "pod", pod.Name)

		if err := cl.Delete(ctx, pod); err != nil {
			logger.Error(err, "could not delete the failed pod", "pod", pod.Name)
		}
		return nil, fmt.Errorf(errMsg)
	}

	logCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := j.kcl.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container: "pause"})
	podLogs, err := req.Stream(logCtx)
	if err != nil {
		return nil, fmt.Errorf("error in opening stream")
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return nil, fmt.Errorf("error in copy information from podLogs to buf")
	}

	err = cl.Delete(ctx, pod)
	if err != nil {
		logger.Error(err, "could not delete the failed pod", "pod", pod.Name)
		return nil, err
	}

	logger.Info("Got raw versions", "json", buf.String())
	res := []elementalv1.ManagedOSVersion{}

	err = json.Unmarshal(buf.Bytes(), &res)
	if err != nil {
		logger.Error(err, "Failed unmarshalling managedOSVersions")
		return nil, err
	}

	meta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
		Type:    elementalv1.ReadyCondition,
		Reason:  elementalv1.GotChannelDataReason,
		Status:  metav1.ConditionFalse,
		Message: "Got valid channel data",
	})

	return res, nil
}

// createSyncerPod creates the pod according to the managed OS version channel configuration
func (j *CustomSyncer) createSyncerPod(ctx context.Context, cl client.Client, ch *elementalv1.ManagedOSVersionChannel) error {
	logger := ctrl.LoggerFrom(ctx)
	logger.Info("Launching syncer pod", "pod", ch.Name)

	mountDir := j.MountPath
	outFile := j.OutputFile
	if mountDir == "" {
		mountDir = defaultMountDir
	}
	if outFile == "" {
		outFile = defaultOutFile
	}

	serviceAccount := false
	err := cl.Create(ctx, &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ch.Name,
			Namespace: ch.Namespace,
			OwnerReferences: []metav1.OwnerReference{
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
			}},
		},
	})
	if err != nil {
		return err
	}

	meta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
		Type:    elementalv1.ReadyCondition,
		Reason:  elementalv1.SyncingReason,
		Status:  metav1.ConditionFalse,
		Message: "On going channel synchronization",
	})
	return nil
}
