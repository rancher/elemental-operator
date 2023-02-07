/*
Copyright © 2022 - 2023 SUSE LLC

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

package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
)

const (
	jobStatusInit       = "Initialized"
	jobStatusStarted    = "Started"
	jobStatusCompleted  = "Completed"
	jobStatusFailed     = "Failed"
	jobStatusNotStarted = "Not Started"
)

type buildImageJobStatus struct {
	Status string `json:"status"`
	URL    string `json:"url"`
}

type buildImageJob struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}

func (i *InventoryServer) apiBuildImage(resp http.ResponseWriter, req *http.Request) error {

	if req.Method == http.MethodPost {
		return i.apiBuildImagePostStart(resp, req)
	} else if req.Method == http.MethodGet {
		return i.apiBuildImageGetStatus(resp, req)
	}

	err := fmt.Errorf("method '%s' is not supported", req.Method)
	http.Error(resp, err.Error(), http.StatusMethodNotAllowed)
	return err
}

func (i *InventoryServer) apiBuildImageGetStatus(resp http.ResponseWriter, req *http.Request) error {
	escapedToken := sanitizeUserInput(path.Base(req.URL.Path))

	logrus.Debugf("Get build-image job status for %s", escapedToken)

	jobStatus := jobStatusNotStarted
	jobDownloadURL := ""
	if reg, err := i.registrationCache.getRegistrationData(escapedToken); err != nil {
		if _, err := i.getMachineRegistration(escapedToken); err != nil {
			logrus.Warnf("Requested build-image status for unexistent MachineRegistration token: %s", escapedToken)
			http.Error(resp, err.Error(), http.StatusBadRequest)
			return err
		}
		logrus.Debug("build-image job was ever started")
	} else {
		jobStatus = reg.buildImageStatus
		jobDownloadURL = reg.downloadURL
	}

	data := buildImageJobStatus{Status: jobStatus, URL: jobDownloadURL}

	if err := json.NewEncoder(resp).Encode(data); err != nil {
		errMsg := fmt.Errorf("cannot marshal build-image data: %w", err)
		http.Error(resp, errMsg.Error(), http.StatusInternalServerError)
		return errMsg
	}

	return nil
}

func sanitizeBuildImageJob(job buildImageJob) buildImageJob {
	job.Token = sanitizeUserInput(job.Token)
	job.URL = sanitizeUserInput(job.URL)
	return job
}

func (i *InventoryServer) apiBuildImagePostStart(resp http.ResponseWriter, req *http.Request) error {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(resp, err.Error(), http.StatusInternalServerError)
		return err
	}

	var job buildImageJob
	if err := json.Unmarshal(body, &job); err != nil {
		errMsg := fmt.Errorf("cannot unmarshal build-image POST data: %w", err)
		http.Error(resp, errMsg.Error(), http.StatusBadRequest)
		return errMsg
	}

	sanitizedJob := sanitizeBuildImageJob(job)

	if _, err := i.getMachineRegistration(sanitizedJob.Token); err != nil {
		http.Error(resp, err.Error(), http.StatusBadRequest)
		return err
	}

	if reg, err := i.registrationCache.getRegistrationData(sanitizedJob.Token); err != nil {
		if reg.buildImageStatus == jobStatusInit || reg.buildImageStatus == jobStatusStarted {
			if reg.buildImageURL == sanitizedJob.URL {
				logrus.Debugf("The build-image job has already started for token %s.", sanitizedJob.Token)

				if err := json.NewEncoder(resp).Encode(sanitizedJob); err != nil {
					errMsg := fmt.Errorf("cannot marshal build-image data: %w", err)
					http.Error(resp, errMsg.Error(), http.StatusInternalServerError)
					return errMsg
				}
				return nil
			}
			logrus.Debugf("Another build-image job is already running with token %s.", sanitizedJob.Token)
			errMsg := fmt.Errorf("a build image task is already running for the MachineRegistration")
			http.Error(resp, errMsg.Error(), http.StatusBadRequest)
			return errMsg
		}
	}

	i.registrationCache.setRegistrationData(sanitizedJob.Token, registrationData{
		buildImageURL:    sanitizedJob.URL,
		buildImageStatus: jobStatusInit,
		downloadURL:      "",
	})

	logrus.Infof("New build-image job queued: seed image:'%s', reg token:'%s'", sanitizedJob.URL, sanitizedJob.Token)
	// start the actual build job here
	go i.doBuildImage(sanitizedJob)

	if err := json.NewEncoder(resp).Encode(sanitizedJob); err != nil {
		errMsg := fmt.Errorf("cannot marshal build-image data: %w", err)
		http.Error(resp, errMsg.Error(), http.StatusInternalServerError)
		return errMsg
	}
	return nil
}

func (i *InventoryServer) doBuildImage(job buildImageJob) {
	// doBuildImage() spawns a pod that:
	// - downloads the base image (job.Url)
	// - retrieves the registration YAML from the registrationToken (job.Token)
	// - generates the elemental image (seed image + registration yaml)
	// - returns the elemental image (url)

	const podName = "build-img"
	const podNamespace = "cattle-elemental-system"

	regURL, err := i.getRegistrationURL(job.Token)
	if err != nil {
		i.setFailedStatus(job.Token, err)
		return
	}

	pod := buildImagePod(podName, podNamespace, job.URL, regURL)
	if err := i.Create(i, pod); err != nil {
		i.setFailedStatus(job.Token, fmt.Errorf("Failed to create build-image pod: %s.", err.Error()))
		return
	}

	logrus.Infof("build image for token %s started.", job.Token)
	if err := i.registrationCache.setDownloadURL(job.Token, ""); err != nil {
		logrus.Errorf("Cannot update build-image download URL for job with token %s: %s", job.Token, err.Error())
	}
	if err := i.registrationCache.setBuildImageStatus(job.Token, jobStatusStarted); err != nil {
		logrus.Errorf("Cannot update build-image status for job with token %s: %s", job.Token, err.Error())
	}

	// TODO: create a Service to expose the built ISO and set properly the download URL

	// TODO: use a watcher and have a timeout
	// TODO: delete the pod on failure
	failedCunter := 12
	watchPod := &corev1.Pod{}
	for {
		failedCunter--
		if err := i.Get(i, client.ObjectKey{Name: podName, Namespace: podNamespace}, watchPod); err != nil {
			if failedCunter == 0 {
				logrus.Errorf("Cannot check %s pod status.", podName)
				i.setBuildStatus(job.Token, jobStatusFailed)
				return
			}
		}
		switch watchPod.Status.Phase {
		case corev1.PodRunning:
			i.setBuildStatus(job.Token, jobStatusCompleted)
			return
		case corev1.PodFailed:
			i.setBuildStatus(job.Token, jobStatusFailed)
			return
		}

		if failedCunter == 0 {
			logrus.Errorf("POD %s timed out.", podName)
			i.setBuildStatus(job.Token, jobStatusFailed)
			return
		}
		time.Sleep(5 * time.Second)
	}
}

func (i *InventoryServer) setBuildStatus(token string, status string) {
	if err := i.registrationCache.setBuildImageStatus(token, status); err != nil {
		logrus.Errorf("Cannot update build-image status for job with token %s: %s.", token, err.Error())
	}
}

func (i *InventoryServer) setFailedStatus(token string, err error) {
	logrus.Errorf("Failed to build img %s: %s.", token, err.Error())
	i.setBuildStatus(token, jobStatusFailed)
}

func (i *InventoryServer) getRegistrationURL(token string) (string, error) {
	mRegistrationList := &elementalv1.MachineRegistrationList{}

	if err := i.List(i, mRegistrationList); err != nil {
		return "", fmt.Errorf("cannot retrieve machine registration list: %w", err)
	}
	regURL := ""
	for _, m := range mRegistrationList.Items {
		if m.Status.RegistrationToken == token {
			regURL = m.Status.RegistrationURL
			break
		}
	}

	if regURL == "" {
		return "", fmt.Errorf("cannot find machine registration with token %s", token)
	}

	return regURL, nil
}

func buildImagePod(name, namespace, seedImgURL, regURL string) *corev1.Pod {
	const buildImage = "registry.opensuse.org/isv/rancher/elemental/stable/teal53/15.4/rancher/elemental-builder-image/5.3:latest"
	// TODO: find a better serveImage
	const serveImage = "quay.io/fgiudici/busybox:latest"
	const volLim = 4 * 1024 * 1024 * 1024 // 4 GiB
	const volRes = 2 * 1024 * 1024 * 1024 // 2 GiB

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app.kubernetes.io/name": name},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:  "build",
					Image: buildImage,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceEphemeralStorage: *resource.NewQuantity(volLim, resource.BinarySI),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceEphemeralStorage: *resource.NewQuantity(volRes, resource.BinarySI),
						},
					},
					Command: []string{"/bin/bash", "-c"},
					Args: []string{
						fmt.Sprintf("%s; %s; %s",
							fmt.Sprintf("curl -Lo base.img %s", seedImgURL),
							fmt.Sprintf("curl -ko reg.yaml %s", regURL),
							"xorriso -indev base.img -outdev /iso/elemental.iso -map reg.yaml /reg.yaml -boot_image any replay"),
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "iso-storage",
							MountPath: "/iso",
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:    "serve",
					Image:   serveImage,
					Command: []string{"/bin/sh", "-c"},
					Args: []string{
						"busybox httpd -f -v -p 80",
					},
					WorkingDir: "/iso",
					Ports: []corev1.ContainerPort{
						{
							Name:          "http-img",
							ContainerPort: 80,
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "iso-storage",
							MountPath: "/iso",
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
			Volumes: []corev1.Volume{
				{
					Name: "iso-storage",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: resource.NewQuantity(volLim, resource.BinarySI),
						},
					},
				},
			},
		},
	}
	return pod
}
