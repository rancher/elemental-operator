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
	"strings"
	"time"

	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

const (
	buildImgResName      = "build-img"
	buildImgResNamespace = "cattle-elemental-system"
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

	logrus.Infof("build-image: received HTTP %s request.", req.Method)
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

	logrus.Debugf("build-image: job status request for %s.", escapedToken)

	jobStatus := jobStatusNotStarted
	jobDownloadURL := ""
	if reg, err := i.registrationCache.getRegistrationData(escapedToken); err != nil {
		if _, err := i.getMachineRegistration(escapedToken); err != nil {
			http.Error(resp, err.Error(), http.StatusBadRequest)
			return err
		}
		logrus.Debug("build-image: job was never started.")
	} else {
		jobStatus = reg.buildImageStatus
		jobDownloadURL = reg.downloadURL
	}

	data := buildImageJobStatus{Status: jobStatus, URL: jobDownloadURL}

	if err := json.NewEncoder(resp).Encode(data); err != nil {
		errMsg := fmt.Errorf("cannot marshal status data: %w", err)
		http.Error(resp, errMsg.Error(), http.StatusInternalServerError)
		return errMsg
	}

	return nil
}

func sanitizeBuildImageJob(job buildImageJob) buildImageJob {
	sanitizedJob := buildImageJob{}
	sanitizedJob.Token = sanitizeUserInput(job.Token)
	sanitizedJob.URL = sanitizeUserInput(job.URL)
	return sanitizedJob
}

func (i *InventoryServer) apiBuildImagePostStart(resp http.ResponseWriter, req *http.Request) error {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(resp, err.Error(), http.StatusInternalServerError)
		return err
	}

	var job buildImageJob
	if err := json.Unmarshal(body, &job); err != nil {
		errMsg := fmt.Errorf("cannot unmarshal POST data: %w", err)
		http.Error(resp, errMsg.Error(), http.StatusBadRequest)
		return errMsg
	}

	sanitizedJob := sanitizeBuildImageJob(job)
	logrus.Debugf("build-image: build request for %s (seed image: %s).", sanitizedJob.Token, sanitizedJob.URL)

	if _, err := i.getMachineRegistration(sanitizedJob.Token); err != nil {
		http.Error(resp, err.Error(), http.StatusBadRequest)
		return err
	}

	if reg, err := i.registrationCache.getRegistrationData(sanitizedJob.Token); err != nil {
		if reg.buildImageStatus == jobStatusInit || reg.buildImageStatus == jobStatusStarted {
			if reg.buildImageURL == sanitizedJob.URL {
				logrus.Infof("build-image: job already started for token %s, skip.", sanitizedJob.Token)

				if err := json.NewEncoder(resp).Encode(sanitizedJob); err != nil {
					errMsg := fmt.Errorf("cannot marshal build-image data: %w", err)
					http.Error(resp, errMsg.Error(), http.StatusInternalServerError)
					return errMsg
				}
				return nil
			}
			logrus.Debugf("build-image: build job already running with token %s and seed image %s.", sanitizedJob.Token, reg.buildImageURL)
			errMsg := fmt.Errorf("a build image task is already running for the MachineRegistration")
			http.Error(resp, errMsg.Error(), http.StatusBadRequest)
			return errMsg
		}
	}

	// Pod lifecycle: currently we allow just one build task at a time that stays around till we don't start another one.
	// TODO: extend to allow multiple build Pods
	// TODO: ensure proper clean-up when the operator is removed or upgraded
	regList := i.registrationCache.getRegistrationKeys()
	for _, token := range regList {
		if status, _ := i.registrationCache.getBuildImageStatus(token); status == jobStatusStarted || status == jobStatusInit {
			errMsg := fmt.Errorf("a build image task is already running for MachineRegistration with token %s", token)
			http.Error(resp, errMsg.Error(), http.StatusServiceUnavailable)
			return errMsg
		} else if status == jobStatusCompleted {
			logrus.Debugf("build-image: delete completed job with token %s.", token)
			i.deleteBuildImagePodService(buildImgResName, buildImgResNamespace)
			_ = i.registrationCache.setBuildImageStatus(token, jobStatusNotStarted)
			break
		}
	}

	// In the unlikely case we have an untracked build Pod (old operator which got upgraded?) let's enforce clean-up
	pod := &corev1.Pod{}
	if err := i.Get(i, client.ObjectKey{Name: buildImgResName, Namespace: buildImgResNamespace}, pod); err == nil {
		logrus.Warnf("build-image: found orphan build pod, enforce clean-up.")
		i.deleteBuildImagePodService(buildImgResName, buildImgResNamespace)
	}

	i.registrationCache.setRegistrationData(sanitizedJob.Token, registrationData{
		buildImageURL:    sanitizedJob.URL,
		buildImageStatus: jobStatusInit,
		downloadURL:      "",
	})

	logrus.Infof("build-image: new job queued (seed image:'%s', reg token:'%s')", sanitizedJob.URL, sanitizedJob.Token)
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

	regURL, err := i.getRegistrationURL(job.Token)
	if err != nil {
		i.setFailedStatus(job.Token, err)
		return
	}

	pod := fillBuildImagePod(buildImgResName, buildImgResNamespace, job.URL, regURL)
	// Creation may fail because we have just asked deletion of a job with the same Pod name, which may take some time.
	// Let's wait and retry if that is the case.
	failedCounter := 15
	for err := i.Create(i, pod); err != nil; {
		watchPod := &corev1.Pod{}
		if failedCounter == 0 {
			i.setFailedStatus(job.Token, fmt.Errorf("failed to create build-image pod: %s", err.Error()))
			return
		}
		if getErr := i.Get(i, client.ObjectKey{Name: buildImgResName, Namespace: buildImgResNamespace}, watchPod); getErr != nil {
			logrus.Debugf("build-image: failed to Get Pod %s:%s: %s.", buildImgResName, buildImgResNamespace, getErr.Error())
			// Eventually the Pod got deleted?
			err = i.Create(i, pod)
			failedCounter = 0
			continue
		}
		if watchPod.DeletionTimestamp == nil {
			logrus.Errorf("build-image: Pod %s:%s already present, stop current build.", buildImgResName, buildImgResNamespace)
			i.setFailedStatus(job.Token, err)
			return
		}
		if failedCounter > 0 {
			failedCounter--
			logrus.Debugf("build-image: wait for old Pod %s:%s to complete deletion.", buildImgResName, buildImgResNamespace)
			time.Sleep(5 * time.Second)
		}
		continue
	}

	logrus.Debugf("build-image: job for token %s started.", job.Token)
	if err := i.registrationCache.setDownloadURL(job.Token, ""); err != nil {
		logrus.Errorf("build-image: cannot update build-image download URL for token %s: %s", job.Token, err.Error())
	}
	if err := i.registrationCache.setBuildImageStatus(job.Token, jobStatusStarted); err != nil {
		logrus.Errorf("build-image: cannot update build-image status for token %s: %s", job.Token, err.Error())
	}

	service := fillBuildImageService(buildImgResName, buildImgResNamespace)
	if err := i.Create(i, service); err != nil {
		i.setFailedStatus(job.Token, fmt.Errorf("failed to create build-image service: %s", err.Error()))
		if err := i.Delete(i, pod); err != nil {
			logrus.Errorf("build-image: failed cleaning up pod %s:%s", pod.Name, pod.Namespace)
		}
		return
	}

	// TODO: use a watcher and have a timeout
	failedCounter = 30
	watchPod := &corev1.Pod{}
	for {
		failedCounter--
		if err := i.Get(i, client.ObjectKey{Name: buildImgResName, Namespace: buildImgResNamespace}, watchPod); err != nil {
			if failedCounter == 0 {
				logrus.Errorf("build-image: cannot check %s pod status.", buildImgResName)
				i.setBuildStatus(job.Token, jobStatusFailed)
				return
			}
		}
		switch watchPod.Status.Phase {
		case corev1.PodRunning:
			svc := &corev1.Service{}
			logrus.Infof("build-image: job %s: Completed.", job.Token)
			i.setBuildStatus(job.Token, jobStatusCompleted)
			if err := i.Get(i, client.ObjectKey{Name: buildImgResName, Namespace: buildImgResNamespace}, svc); err != nil {
				logrus.Error("build-image: failed to retrieve iso-image URL.")
				return
			}
			rancherURL, err := i.getRancherServerAddress()
			if err != nil {
				logrus.Errorf("build-image: failed to retrieve iso-image URL: %s.", err.Error())
			}

			if err := i.registrationCache.setDownloadURL(job.Token, fmt.Sprintf("http://%s:%d/elemental.iso", rancherURL, svc.Spec.Ports[0].NodePort)); err != nil {
				logrus.Errorf("build-image: failed to set download URL: %s.", err.Error())
			} else {
				logrus.Debug("build-image: download URL set.")
			}
			return
		case corev1.PodFailed:
			logrus.Infof("build-image: job %s: Failed.", job.Token)
			i.setBuildStatus(job.Token, jobStatusFailed)
			return
		}

		if failedCounter == 0 {
			logrus.Errorf("build-image: job %s timed out.", job.Token)
			i.setBuildStatus(job.Token, jobStatusFailed)
			return
		}
		time.Sleep(10 * time.Second)
	}
}

func (i *InventoryServer) setBuildStatus(token string, status string) {
	if err := i.registrationCache.setBuildImageStatus(token, status); err != nil {
		logrus.Errorf("build-image: cannot update build-image status for token %s: %s.", token, err.Error())
	}
}

func (i *InventoryServer) setFailedStatus(token string, err error) {
	logrus.Errorf("build-image: failed to build img %s: %s.", token, err.Error())
	i.setBuildStatus(token, jobStatusFailed)
}

func (i *InventoryServer) getRancherServerAddress() (string, error) {
	setting := &managementv3.Setting{}
	if err := i.Get(i, types.NamespacedName{Name: "server-url"}, setting); err != nil {
		return "", fmt.Errorf("failed to get server url setting: %w", err)
	}

	if setting.Value == "" {
		return "", fmt.Errorf("server-url is not set")
	}

	return strings.TrimPrefix(setting.Value, "https://"), nil
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

func fillBuildImagePod(name, namespace, seedImgURL, regURL string) *corev1.Pod {
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

func fillBuildImageService(name, namespace string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": name},
			Ports: []corev1.ServicePort{
				{
					Name:     "elemental-build-image",
					Protocol: corev1.ProtocolTCP,
					Port:     80,
				},
			},
			Type: corev1.ServiceTypeNodePort,
		},
	}

	return service
}

func (i *InventoryServer) deleteBuildImagePodService(name, namespace string) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	if err := i.Delete(i, pod); err != nil {
		logrus.Errorf("build-image: cannot delete Pod %s:%s.", pod.Name, pod.Namespace)
	}
	logrus.Debugf("build-image: Pod %s:%s deleted.", pod.Name, pod.Namespace)
	if err := i.Delete(i, svc); err != nil {
		logrus.Errorf("build-image: cannot delete Service %s:%s.", svc.Name, svc.Namespace)
	}
	logrus.Debugf("build-image: Service %s:%s deleted.", svc.Name, svc.Namespace)
}
