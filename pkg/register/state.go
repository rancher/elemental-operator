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
	"path/filepath"

	"time"

	"github.com/pkg/errors"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/twpayne/go-vfs"
	"gopkg.in/yaml.v3"
)

type State struct {
	InitialRegistration time.Time `yaml:"initialRegistration,omitempty"`
	LastUpdate          time.Time `yaml:"lastUpdate,omitempty"`
	EmulatedTPM         bool      `yaml:"emulatedTPM,omitempty"`
	EmulatedTPMSeed     int64     `yaml:"emulatedTPMSeed,omitempty"`
}

func (s *State) IsUpdatable() bool {
	return !s.InitialRegistration.IsZero()
}

func (s *State) HasLastUpdateElapsed(suppress time.Duration) bool {
	return time.Now().After(s.LastUpdate.Add(suppress))
}

type StateHandler interface {
	Load() (State, error)
	Save(State) error
}

var errDecodingState = errors.New("decoding state")

var _ StateHandler = (*filesystemStateHandler)(nil)

func NewFileStateHandler(fs vfs.FS, stateFilePath string) StateHandler {
	return &filesystemStateHandler{
		fs:            fs,
		stateFilePath: stateFilePath,
	}
}

type filesystemStateHandler struct {
	fs            vfs.FS
	stateFilePath string
}

func (h *filesystemStateHandler) Load() (State, error) {
	stateFile := h.stateFilePath
	file, err := h.fs.Open(stateFile)
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
		return State{}, fmt.Errorf("%w from file '%s': %s", errDecodingState, stateFile, err)
	}
	if err := file.Close(); err != nil {
		return State{}, fmt.Errorf("closing file '%s': %w", stateFile, err)
	}
	return state, nil
}

func (h *filesystemStateHandler) Save(state State) error {
	directory := filepath.Dir(h.stateFilePath)
	if _, err := h.fs.Stat(directory); os.IsNotExist(err) {
		log.Debugf("Registration config dir '%s' does not exist. Creating now.", directory)
		if err := vfs.MkdirAll(h.fs, directory, 0700); err != nil {
			return fmt.Errorf("creating registration config directory: %w", err)
		}
	}
	stateFile := h.stateFilePath
	file, err := h.fs.Create(stateFile)
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
