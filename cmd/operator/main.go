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

package main

import (
	displayCmd "github.com/rancher/elemental-operator/cmd/operator/display"
	operatorCmd "github.com/rancher/elemental-operator/cmd/operator/operator"
	registerCmd "github.com/rancher/elemental-operator/cmd/operator/register"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func main() {
	cmd := &cobra.Command{
		Use:   "elemental-operator",
		Short: "Elemental Kubernetes Operator",
	}

	cmd.AddCommand(
		operatorCmd.NewOperatorCommand(),
		registerCmd.NewRegisterCommand(),
		displayCmd.NewDisplayCommand())

	if err := cmd.Execute(); err != nil {
		logrus.Fatalln(err)
	}
}
