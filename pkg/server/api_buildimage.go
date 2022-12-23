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

package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"

	"github.com/sirupsen/logrus"
)

const (
	jobStatusStarted = "Started"
	//	jobStatusCompleted  = "Completed"
	jobStatusFailed     = "Failed"
	jobStatusNotStarted = "Not Started"
)

type buildImageJobStatus struct {
	Status string `json:"status"`
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
	if reg, err := i.registrationCache.getRegistrationData(escapedToken); err != nil {
		if _, err := i.getMachineRegistration(escapedToken); err != nil {
			logrus.Warnf("Requested build-image status for unexistent MachineRegistration token: %s", escapedToken)
			http.Error(resp, err.Error(), http.StatusBadRequest)
			return err
		}
		logrus.Debug("build-image job was ever started")
	} else {
		jobStatus = reg.buildImageStatus
	}

	data := buildImageJobStatus{Status: jobStatus}

	if err := json.NewEncoder(resp).Encode(data); err != nil {
		errMsg := fmt.Errorf("cannot marshal build-image data: %w", err)
		http.Error(resp, errMsg.Error(), http.StatusInternalServerError)
		return errMsg
	}

	return nil
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

	job.Token = sanitizeUserInput(job.Token)
	job.URL = sanitizeUserInput(job.URL)

	if _, err := i.getMachineRegistration(job.Token); err != nil {
		http.Error(resp, err.Error(), http.StatusBadRequest)
		return err
	}

	if reg, err := i.registrationCache.getRegistrationData(job.Token); err != nil {
		if reg.buildImageStatus == jobStatusStarted {
			if reg.buildImageURL == job.URL {
				logrus.Debugf("Same job already started, nothing to do")

				if err := json.NewEncoder(resp).Encode(job); err != nil {
					errMsg := fmt.Errorf("cannot marshal build-image data: %w", err)
					http.Error(resp, errMsg.Error(), http.StatusInternalServerError)
					return errMsg
				}
				return nil
			}
			errMsg := fmt.Errorf("a build image task is already running for the MachineRegistration")
			http.Error(resp, errMsg.Error(), http.StatusBadRequest)
			return errMsg
		}
	}

	i.registrationCache.setRegistrationData(job.Token, registrationData{
		buildImageURL:    job.URL,
		buildImageStatus: jobStatusStarted,
	})

	logrus.Infof("New build-image job queued: seed image:'%s', reg token:'%s'", job.URL, job.Token)
	// start the actual build job here
	go i.draftDoBuildImage(job)

	if err := json.NewEncoder(resp).Encode(job); err != nil {
		errMsg := fmt.Errorf("cannot marshal build-image data: %w", err)
		http.Error(resp, errMsg.Error(), http.StatusInternalServerError)
		return errMsg
	}
	return nil
}

func (i *InventoryServer) draftDoBuildImage(job buildImageJob) {
	// This should spawn a pod that will:
	// - download the base image (job.Url)
	// - retrieve the registration YAML from the registrationToken (job.Token)
	// - generate the seed image (base image + registration yaml)
	// - return the seed image (url? shared volume?)
	// Here we should just update the status then.
	// For now, it will always fail.
	logrus.Infof("build-image for token %s failed (build job not supported yet).", job.Token)
	if err := i.registrationCache.setBuildImageStatus(job.Token, jobStatusFailed); err != nil {
		logrus.Errorf("Cannot update build-image status for job with token %s: %s", job.Token, err.Error())
	}
}
