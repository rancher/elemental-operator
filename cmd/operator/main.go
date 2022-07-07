package main

import (
	display_cmd "github.com/rancher/elemental-operator/cmd/operator/display"
	operator_cmd "github.com/rancher/elemental-operator/cmd/operator/operator"
	register_cmd "github.com/rancher/elemental-operator/cmd/operator/register"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func main() {
	cmd := &cobra.Command{
		Use:   "elemental-operator",
		Short: "Elemental Kubernetes Operator",
	}

	cmd.AddCommand(
		operator_cmd.NewOperatorCommand(),
		register_cmd.NewRegisterCommand(),
		display_cmd.NewDisplayCommand())

	if err := cmd.Execute(); err != nil {
		logrus.Fatalln(err)
	}
}
