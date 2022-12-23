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
	"fmt"
	"sync"
)

type registrationData struct {
	buildImageURL    string
	buildImageStatus string
}
type registrationCache struct {
	*sync.Mutex
	registrations map[string]registrationData
}

func (rc *registrationCache) getRegistrationData(token string) (registrationData, error) {
	rc.Lock()
	defer rc.Unlock()

	if _, ok := rc.registrations[token]; !ok {
		return registrationData{}, fmt.Errorf("item not found")
	}
	return rc.registrations[token], nil
}

func (rc *registrationCache) setRegistrationData(token string, data registrationData) {
	rc.Lock()
	defer rc.Unlock()

	rc.registrations[token] = data
}

func (rc *registrationCache) setBuildImageStatus(token, status string) error {
	rc.Lock()
	defer rc.Unlock()

	reg, ok := rc.registrations[token]
	if !ok {
		return fmt.Errorf("item not found")
	}

	reg.buildImageStatus = status
	rc.registrations[token] = reg
	return nil
}
