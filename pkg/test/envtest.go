package test

import (
	"errors"
	"path"
	goruntime "runtime"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func init() {
	utilruntime.Must(elementalv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(managementv3.AddToScheme(scheme.Scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme.Scheme))
}

func StartEnvTest(testEnv *envtest.Environment) (*rest.Config, client.Client, error) {
	// Get the root of the current file to use in CRD paths.
	_, filename, _, _ := goruntime.Caller(0) //nolint:dogsled
	root := path.Join(path.Dir(filename), "..", "..", "..", "elemental-operator")

	testEnv.CRDs = []*apiextensionsv1.CustomResourceDefinition{
		fakeSettingCRD,
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

	cl, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, nil, err
	}

	return cfg, cl, nil
}

func StopEnvTest(testEnv *envtest.Environment) error {
	return testEnv.Stop()
}