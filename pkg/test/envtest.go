/*
Copyright © 2022 - 2025 SUSE LLC

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

package test

import (
	"errors"
	"path"
	goruntime "runtime"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	fleetv1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(elementalv1.AddToScheme(scheme))
	utilruntime.Must(managementv3.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(upgradev1.AddToScheme(scheme))
	utilruntime.Must(fleetv1.AddToScheme(scheme))
}

func StartEnvTest(testEnv *envtest.Environment) (*rest.Config, client.Client, error) {
	// Get the root of the current file to use in CRD paths.
	_, filename, _, _ := goruntime.Caller(0) //nolint:dogsled
	root := path.Join(path.Dir(filename), "..", "..", "..", "elemental-operator")

	testEnv.CRDs = []*apiextensionsv1.CustomResourceDefinition{
		fakeSettingCRD,
		fakeMachineCRD,
		fakeBundleCRD,
	}
	testEnv.CRDDirectoryPaths = []string{
		path.Join(root, "config", "crd", "bases"),
	}
	testEnv.ErrorIfCRDPathMissing = true

	cfg, err := testEnv.Start()
	if err != nil {
		return nil, nil, err
	}

	if cfg == nil {
		return nil, nil, errors.New("envtest.Environment.Start() returned nil config")
	}

	cl, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, err
	}

	return cfg, cl, nil
}

func StopEnvTest(testEnv *envtest.Environment) error {
	return testEnv.Stop()
}
