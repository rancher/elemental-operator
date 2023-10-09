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

package cleanup

import (
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	client "sigs.k8s.io/controller-runtime/pkg/client"
	runtimeconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
)

// +kubebuilder:rbac:groups=elemental.cattle.io,resources=machineinventories,verbs=get;list;update;patch

var (
	scheme     = runtime.NewScheme()
	cleanupLog = ctrl.Log.WithName("cleanup")
)

func NewCleanupCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Removes existing finalizers so CRDs deletion will not leave resources pending on finalizers management",
		Args:  cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			cleanupRun()
		},
	}

	return cmd
}

func cleanupRun() {
	cleanupLog.Info("starting registration")

	ctx := ctrl.SetupSignalHandler()

	restConfig, err := runtimeconfig.GetConfig()
	if err != nil {
		cleanupLog.Error(err, "failed to find kubeconfig")
		os.Exit(1)
	}

	err = elementalv1.AddToScheme(scheme)
	if err != nil {
		cleanupLog.Error(err, "failed setting the scheme")
		os.Exit(1)
	}

	cl, err := client.New(restConfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		cleanupLog.Error(err, "error building restconfig")
		os.Exit(1)
	}

	inventoryList := &elementalv1.MachineInventoryList{}

	err = cl.List(ctx, inventoryList, &client.ListOptions{})
	if err != nil {
		cleanupLog.Error(err, "failed getting MachineInventoryList")
		os.Exit(1)
	}

	var errs []error
	cleanupLog.Info("Cleaning MachineInventory resources")
	for _, i := range inventoryList.Items {
		cleanupLog.Info("Removing finalizers", "name", i.Name)
		patchBase := client.MergeFrom(i.DeepCopy())
		i.Finalizers = []string{}
		err = cl.Patch(ctx, &i, patchBase)
		if err != nil {
			cleanupLog.Error(err, "failed patching MachineInventory", "name", i.Name)
			errs = append(errs, err)
		}
	}
	if errs != nil {
		cleanupLog.Error(err, "failed patching some resources")
		os.Exit(1)
	}
}
