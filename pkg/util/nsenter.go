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

package util

import (
	"fmt"
	"os"

	"github.com/rancher/elemental-operator/pkg/log"
	"k8s.io/utils/exec"
	"k8s.io/utils/nsenter"
)

type NsEnter interface {
	IsSystemShuttingDown(hostDir string) (bool, error)
	Reboot(hostDir string)
}

func NewNsEnter() NsEnter {
	return &nsEnter{}
}

var _ NsEnter = (*nsEnter)(nil)

type nsEnter struct{}

func (n *nsEnter) IsSystemShuttingDown(hostDir string) (bool, error) {
	ex := exec.New()
	nsEnter, err := nsenter.NewNsenter(hostDir, ex)
	if err != nil {
		return false, fmt.Errorf("initializing nsenter: %w", err)
	}
	cmd := nsEnter.Command("systemctl", "is-system-running")
	cmd.SetStdin(os.Stdin)
	cmd.SetStderr(os.Stderr)
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("running: systemctl is-system-running: %w", err)
	}
	if string(output) == "stopping" {
		return true, nil
	}
	return false, nil
}

func (n *nsEnter) Reboot(hostDir string) {
	ex := exec.New()
	nsEnter, err := nsenter.NewNsenter(hostDir, ex)
	if err != nil {
		log.Errorf("Coult not initialize nsenter: %s", err.Error())
	}
	cmd := nsEnter.Command("reboot")
	cmd.SetStdin(os.Stdin)
	cmd.SetStderr(os.Stderr)
	cmd.SetStdout(os.Stdout)
	if err := cmd.Run(); err != nil {
		log.Errorf("Could not reboot: %s", err)
	}
}
