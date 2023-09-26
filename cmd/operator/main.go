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

package main

import (
	"github.com/spf13/cobra"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"

	displaycmd "github.com/rancher/elemental-operator/cmd/operator/display"
	downloadcmd "github.com/rancher/elemental-operator/cmd/operator/download"
	operatorcmd "github.com/rancher/elemental-operator/cmd/operator/operator"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/version"
)

func main() {
	logger := klogr.New()
	ctrl.SetLogger(logger)

	cmd := &cobra.Command{
		Use:   "elemental-operator",
		Short: "Elemental Kubernetes Operator",
	}

	cmd.AddCommand(
		operatorcmd.NewOperatorCommand(),
		displaycmd.NewDisplayCommand(),
		downloadcmd.NewDownloadCommand(),
		NewVersionCommand(),
	)

	if err := cmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}

func NewVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print operator version",
		Run: func(_ *cobra.Command, _ []string) {
			log.Infof("Operator version %s, commit %s, commit date %s", version.Version, version.Commit, version.CommitDate)
		},
	}
	return cmd
}
