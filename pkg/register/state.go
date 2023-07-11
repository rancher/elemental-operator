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

package register

import (
	"fmt"
	"os"
	"time"

	"github.com/rancher/elemental-operator/pkg/log"
	"gopkg.in/yaml.v3"
)

type State struct {
	initialRegistration time.Time `yaml:"initialRegistration,omitempty"`
	lastUpdate          time.Time `yaml:"lastUpdate,omitempty"`
	emulatedTPM         bool      `yaml:"emulatedTPM,omitempty"`
	emulatedTPMSeed     int64     `yaml:"emulatedTPMSeed,omitempty"`
}

func (s *State) IsUpdatable() bool {
	return !s.initialRegistration.IsZero()
}

type StateHandler interface {
	Load() (State, error)
	Save(State) error
}

var _ StateHandler = (*filesystemStateHandler)(nil)

func NewFileStateHandler(directory string) StateHandler {
	return &filesystemStateHandler{directory: directory}
}

type filesystemStateHandler struct {
	directory string
}

func (h *filesystemStateHandler) getStateFullPath() string {
	const stateFile = "state.yaml"
	return fmt.Sprintf("%s/%s", h.directory, stateFile)
}

func (h *filesystemStateHandler) Load() (State, error) {
	stateFile := h.getStateFullPath()
	file, err := os.Open(stateFile)
	defer file.Close()
	if os.IsNotExist(err) {
		log.Debugf("Could not find state file in '%s'. Assuming initial registration needs to happen.", stateFile)
		return State{}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("loading registration state file '%s': %w", stateFile, err)
	}
	dec := yaml.NewDecoder(file)
	var state State
	if err := dec.Decode(&state); err != nil {
		return State{}, fmt.Errorf("decoding registration to file '%s': %w", stateFile, err)
	}
	return state, nil
}

func (h *filesystemStateHandler) Save(state State) error {
	if _, err := os.Stat(h.directory); os.IsNotExist(err) {
		log.Debugf("Registration config dir '%s' does not exist. Creating now.", h.directory)
		if err := os.MkdirAll(h.directory, 0700); err != nil {
			return fmt.Errorf("creating registration config directory: %w", err)
		}
	}
	stateFile := h.getStateFullPath()
	file, err := os.Create(stateFile)
	if err != nil {
		return fmt.Errorf("creating registration state file: %w", err)
	}
	enc := yaml.NewEncoder(file)
	if err := enc.Encode(state); err != nil {
		return fmt.Errorf("writing RegistrationState to file '%s': %w", stateFile, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("closing encoder: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing file '%s': %w", stateFile, err)
	}
	return nil
}
