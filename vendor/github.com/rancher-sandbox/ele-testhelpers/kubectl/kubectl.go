//nolint:goheader
/*
Copyright © 2021 Cloudfoundry (https://github.com/cloudfoundry-incubator/quarks-utils)
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

package kubectl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"

	"github.com/onsi/gomega"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"

	wait "github.com/rancher-sandbox/ele-testhelpers/helpers"
)

const (
	helmCmd    = "helm"
	kubeCtlCmd = "kubectl"
)

// Kubectl is used as a command to test e2e tests
type Kubectl struct {
	Log          *zap.SugaredLogger
	Namespace    string
	PollTimeout  time.Duration
	PollInterval time.Duration
}

// New returns a new Kubectl command
func New() *Kubectl {
	return &Kubectl{
		Namespace:    "",
		PollTimeout:  300 * time.Second,
		PollInterval: 500 * time.Millisecond,
	}
}

// RunCommandWithCheckString runs the command specified helper in the container
func (k *Kubectl) RunCommandWithCheckString(namespace string, podName string, commandInPod string, result string) error {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "exec", podName, "--", "sh", "-c", commandInPod)
	if err != nil {
		return err
	}
	if strings.Contains(string(out), result) {
		return nil
	}
	return errors.Errorf("'%s' not found in output '%s'", result, string(out))
}

// GetPodNames returns the names of the pods matching the selector
func (k *Kubectl) GetPodNames(namespace string, selector string) ([]string, error) {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "get", "pod",
		"-l", selector,
		"-o", "jsonpath={.items[*].metadata.name}")
	if err != nil {
		return []string{}, err
	}
	names := strings.Split(string(out), " ")
	var namesR []string
	for _, n := range names {
		if n != "" {
			namesR = append(namesR, n)
		}
	}

	return namesR, nil
}

// WaitForNamespaceWithPod blocks until pods matching the selector are available in the specified namespace. It fails after the timeout.
func (k *Kubectl) WaitForNamespaceWithPod(namespace string, labelName string) error {
	return wait.PollImmediate(k.PollInterval, k.PollTimeout, func() (bool, error) {
		return k.NamespaceWithReadyPod(namespace, labelName)
	})
}

// NamespaceWithReadyPod returns true if pods by that label are present in the given namespace
func (k *Kubectl) NamespaceWithReadyPod(namespace string, labelName string) (bool, error) {
	pods, err := k.GetPodNames(namespace, labelName)
	if err != nil {
		return false, err
	}
	for _, p := range pods {
		e, _ := k.PodStatus(namespace, p)
		if e == nil || e.ContainerStatuses == nil || len(e.ContainerStatuses) == 0 {
			return false, nil
		}
		for _, s := range e.ContainerStatuses {
			if !s.Ready {
				return false, nil
			}
		}
	}
	return true, nil
}

// WaitNamespacePodsDelete blocks until pods are still available in the given namespace. It fails after the timeout.
func (k *Kubectl) WaitNamespacePodsDelete(namespace string) error {
	return wait.PollImmediate(k.PollInterval, k.PollTimeout, func() (bool, error) {
		return k.checkNamespacePodsDeleted(namespace)
	})
}

// checkNamespacePodsDeleted checks if there is no more pod available in the given namespace.
func (k *Kubectl) checkNamespacePodsDeleted(namespace string) (bool, error) {
	pods, err := k.GetPodNames(namespace, "")
	if err != nil {
		return false, err
	}

	if len(pods) <= 0 {
		return true, nil
	}
	return false, nil
}

// WaitForNamespaceDelete blocks while the namespace is available. It fails after the timeout.
func (k *Kubectl) WaitForNamespaceDelete(namespace string) error {
	return wait.PollImmediate(k.PollInterval, k.PollTimeout, func() (bool, error) {
		return k.checkNamespaceDeleted(namespace)
	})
}

// checkNamespaceDeleted checks if the namespace is terminated
func (k *Kubectl) checkNamespaceDeleted(namespace string) (bool, error) {
	phase, err := GetData(namespace, "namespace", namespace, `jsonpath={.status.phase}`)
	if err != nil {
		return false, err
	}
	if string(phase) == "" {
		return true, nil
	}
	return false, nil
}

// WaitForPod blocks until the pod is available. It fails after the timeout.
func (k *Kubectl) WaitForPod(namespace string, labelName string, podName string) error {
	return wait.PollImmediate(k.PollInterval, k.PollTimeout, func() (bool, error) {
		return k.PodExists(namespace, labelName, podName)
	})
}

// WaitForPodDelete blocks while the pod is available. It fails after the timeout.
func (k *Kubectl) WaitForPodDelete(namespace string, podName string) error {
	return wait.PollImmediate(k.PollInterval, k.PollTimeout, func() (bool, error) {
		return k.checkPodDeleted(namespace, podName)
	})
}

// checkPodTerminateLabelFilter checks if the pod status is terminated
func (k *Kubectl) checkPodDeleted(namespace string, name string) (bool, error) {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "get", "pod", name)
	msg := string(out)
	if err != nil {
		if strings.Contains(msg, "Error from server (NotFound)") {
			return true, nil
		}
		return false, errors.Wrapf(err, "Kubectl get pod '%s' failed: %s", name, msg)
	}
	return false, nil
}

// PodExists returns true if the pod by that label is present
func (k *Kubectl) PodExists(namespace string, labelName string, podName string) (bool, error) {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "get", "pod", "-l", labelName)
	if err != nil {
		return false, errors.Wrapf(err, "Getting pod %s failed. %s", labelName, string(out))
	}
	if strings.Contains(string(out), podName) {
		return true, nil
	}
	return false, nil
}

// PodStatus returns the status if the pod by that label is present
func (k *Kubectl) PodStatus(namespace string, podName string) (*PodStatus, error) {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "get", "pod", podName, "-o", "json")
	if err != nil {
		return nil, errors.Wrapf(err, "Getting pod %s failed. %s", podName, string(out))
	}
	var pod Pod
	err = json.Unmarshal(out, &pod)
	if err != nil {
		return nil, errors.Wrapf(err, "Invalid json '%s': %s", string(out), err.Error())
	}
	return &pod.Status, nil
}

// WaitForService blocks until the service is available. It fails after the timeout.
func (k *Kubectl) WaitForService(namespace string, serviceName string) error {
	return wait.PollImmediate(k.PollInterval, k.PollTimeout, func() (bool, error) {
		return k.ServiceExists(namespace, serviceName)
	})
}

// ServiceExists returns true if the pod by that name is in state running
func (k *Kubectl) ServiceExists(namespace string, serviceName string) (bool, error) {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "get", "service", serviceName)
	if err != nil {
		return false, errors.Wrapf(err, "Getting service %s failed. %s", serviceName, string(out))
	}
	if strings.Contains(string(out), serviceName) {
		return true, nil
	}
	return false, nil
}

// Exists returns true if the resource by that name exists
func (k *Kubectl) Exists(namespace, resource, name string) (bool, error) {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "get", resource, name)
	if err != nil {
		return false, errors.Wrapf(err, "Getting %s %s failed. %s", resource, name, string(out))
	}
	if strings.Contains(string(out), name) {
		return true, nil
	}
	return false, nil
}

// WaitForSecret blocks until the secret is available. It fails after the timeout.
func (k *Kubectl) WaitForSecret(namespace string, secretName string) error {
	return wait.PollImmediate(k.PollInterval, k.PollTimeout, func() (bool, error) {
		return k.SecretExists(namespace, secretName)
	})
}

// SecretExists returns true if the pod by that name is in state running
func (k *Kubectl) SecretExists(namespace string, secretName string) (bool, error) {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "get", "secret", secretName)
	if err != nil {
		if strings.Contains(string(out), "Error from server (NotFound)") {
			return false, nil
		}
		return false, errors.Wrapf(err, "Getting secret %s failed. %s", secretName, string(out))
	}
	if strings.Contains(string(out), secretName) {
		return true, nil
	}
	return false, nil
}

// WaitForPVC blocks until the pvc is available. It fails after the timeout.
func (k *Kubectl) WaitForPVC(namespace string, pvcName string) error {
	return wait.PollImmediate(k.PollInterval, k.PollTimeout, func() (bool, error) {
		return k.pvcExists(namespace, pvcName)
	})
}

// pvcExists returns true if the pvc by that name exists
func (k *Kubectl) pvcExists(namespace string, pvcName string) (bool, error) {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "get", "pvc", pvcName)
	if err != nil {
		if strings.Contains(string(out), "no matching resources found") {
			return false, nil
		}
		return false, errors.Wrapf(err, "Getting pvc %s failed. %s", pvcName, string(out))
	}
	if strings.Contains(string(out), pvcName) {
		return true, nil
	}
	return false, nil
}

// Wait waits for the condition on the resource using kubectl command
func (k *Kubectl) Wait(namespace string, requiredStatus string, resourceName string, customTimeout time.Duration) error {
	err := wait.PollImmediate(k.PollInterval, customTimeout, func() (bool, error) {
		return k.checkWait(namespace, requiredStatus, resourceName)
	})

	if err != nil {
		return errors.Wrapf(err, string(debug.Stack()))
	}

	return nil
}

// checkWait check's if the condition is satisfied
func (k *Kubectl) checkWait(namespace string, requiredStatus string, resourceName string) (bool, error) {
	cmd := exec.Command("kubectl", "--namespace", namespace, "wait", "--for=condition="+requiredStatus, resourceName, "--timeout=60s")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "Error from server (NotFound)") {
			return false, nil
		}
		return false, errors.Wrapf(err, "Kubectl wait failed for %s with status %s. %s", resourceName, requiredStatus, string(out))
	}
	return true, nil
}

// WaitLabelFilter waits for the condition on the resource based on label using kubectl command
func (k *Kubectl) WaitLabelFilter(namespace string, requiredStatus string, resourceName string, labelName string) error {
	if requiredStatus == "complete" {
		return wait.PollImmediate(k.PollInterval, k.PollTimeout, func() (bool, error) {
			return k.checkPodCompleteLabelFilter(namespace, labelName)
		})
	} else if requiredStatus == "terminate" {
		return wait.PollImmediate(k.PollInterval, k.PollTimeout, func() (bool, error) {
			return k.checkPodTerminateLabelFilter(namespace, labelName)
		})
	} else if requiredStatus == "ready" {
		return wait.PollImmediate(k.PollInterval, k.PollTimeout, func() (bool, error) {
			return k.checkPodReadyLabelFilter(namespace, resourceName, labelName, requiredStatus)
		})
	}
	return nil
}

// checkPodReadyLabelFilter checks is the pod status is completed
func (k *Kubectl) checkPodReadyLabelFilter(namespace string, resourceName string, labelName string, requiredStatus string) (bool, error) {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "wait", resourceName, "-l", labelName, "--for=condition="+requiredStatus)
	if strings.Contains(string(out), "no matching resources found") {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrapf(err, "Kubectl wait failed for %s with status %s. %s", resourceName, requiredStatus, string(out))
	}
	return true, nil
}

// checkPodCompleteLabelFilter checks is the pod status is completed
func (k *Kubectl) checkPodCompleteLabelFilter(namespace string, labelName string) (bool, error) {
	exitCodeTemplate := "go-template=\"{{(index (index .items 0).status.containerStatuses 0).state.terminated.exitCode}}\""
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "get", "pod", "-l", labelName, "-o", exitCodeTemplate)
	if err != nil {
		return false, nil
	}
	if string(out) == "\"0\"" {
		return true, nil
	}
	return false, nil
}

// checkPodTerminateLabelFilter checks is the pod status is terminated
func (k *Kubectl) checkPodTerminateLabelFilter(namespace string, labelName string) (bool, error) {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "get", "pod", "-l", labelName)
	if err != nil {
		return false, errors.Wrapf(err, "Kubectl get pod failed with label %s failed. %s", labelName, string(out))

	}
	if strings.HasPrefix(string(out), "No resources found") {
		return true, nil
	}
	return false, nil
}

// CreateRoleBinding Create a new rolebinding in a namespace from a cluster role
func (k *Kubectl) CreateRoleBinding(namespace string, clusterrole, serviceaccount, role string) error {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "create", "rolebinding", "--clusterrole", clusterrole, "--serviceaccount", serviceaccount, role)
	if err != nil {
		return errors.Wrapf(err, "Kubectl create rolebinding failed with role %s failed. %s", clusterrole, string(out))

	}

	return nil
}

// CreateServiceAccount Create a new serviceaccount in a namespace
func (k *Kubectl) CreateServiceAccount(namespace string, serviceaccount string) error {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "create", "serviceaccount", serviceaccount)
	if err != nil {
		return errors.Wrapf(err, "Kubectl create serviceaccount with %s failed. %s", serviceaccount, string(out))

	}

	return nil
}

// DeleteRoleBinding Deletes a rolebinding in a namespace
func (k *Kubectl) DeleteRoleBinding(namespace string, role string) error {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "delete", "rolebinding", role)
	if err != nil {
		return errors.Wrapf(err, "Kubectl delete rolebinding failed with role %s failed. %s", role, string(out))

	}

	return nil
}

// DeleteServiceAccount Deletes a serviceaccount in a namespace
func (k *Kubectl) DeleteServiceAccount(namespace string, serviceaccount string) error {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "delete", "serviceaccount", serviceaccount)
	if err != nil {
		return errors.Wrapf(err, "Kubectl delete serviceaccount with %s failed. %s", serviceaccount, string(out))

	}

	return nil
}

// CreateNamespace create the namespace using kubectl command
func CreateNamespace(name string) error {
	_, err := runBinary(kubeCtlCmd, "create", "namespace", name)
	if err != nil {
		return errors.Wrapf(err, "Deleting namespace %s failed", name)
	}
	return nil
}

// DeleteNamespace removes existing ns
func DeleteNamespace(ns string) error {
	fmt.Printf("Cleaning up namespace %s \n", ns)

	_, err := runBinary(kubeCtlCmd, "delete", "--wait=false", "--ignore-not-found", "--grace-period=30", "namespace", ns)
	if err != nil {
		return errors.Wrapf(err, "Deleting namespace %s failed", ns)
	}

	return nil
}

// Create creates the resource using kubectl command
func Create(namespace string, yamlFilePath string) error {
	_, err := runBinary(kubeCtlCmd, "--namespace", namespace, "create", "-f", yamlFilePath)
	if err != nil {
		return errors.Wrapf(err, "creating yaml spec %s failed", yamlFilePath)
	}
	return nil
}

// CreateSecretFromLiteral creates a generic type secret using kubectl command
func CreateSecretFromLiteral(namespace string, secretName string, literalValues map[string]string) error {
	args := []string{"--namespace", namespace, "create", "secret", "generic", secretName}

	for key, value := range literalValues {
		args = append(args, fmt.Sprintf("--from-literal=%s=%s", key, value))
	}

	_, err := runBinary(kubeCtlCmd, args...)
	if err != nil {
		return errors.Wrapf(err, "creating secret %s failed from literal value", secretName)
	}
	return nil
}

// DeleteSecret deletes the namespace using kubectl command
func DeleteSecret(namespace string, secretName string) error {
	_, err := runBinary(kubeCtlCmd, "--namespace", namespace, "delete", "secret", secretName, "--ignore-not-found")
	return err
}

// Apply updates the resource using kubectl command
func Apply(namespace string, yamlFilePath string) error {
	_, err := runBinary(kubeCtlCmd, "--namespace", namespace, "apply", "-f", yamlFilePath)
	return err
}

// PatchNamespace patche the namespace resource using kubectl command
func PatchNamespace(name string, patch string) error {
	_, err := runBinary(kubeCtlCmd,
		"patch", "namespace", name,
		"--type=json", "-p", patch)
	return err
}

// Delete creates the resource using kubectl command
func Delete(namespace string, yamlFilePath string) error {
	_, err := runBinary(kubeCtlCmd, "--namespace", namespace, "delete", "-f", yamlFilePath)
	return err
}

// DeleteResource deletes the resource using kubectl command
func DeleteResource(namespace string, resourceName string, name string) error {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "delete", resourceName, name)
	if err != nil {
		if strings.Contains(string(out), "Error from server (NotFound)") {
			return nil
		}
		return errors.Wrapf(err, "deleting resource %s failed %s", resourceName, string(out))
	}
	return nil
}

// DeleteLabelFilter deletes the resource based on label using kubectl command
func DeleteLabelFilter(namespace string, resourceName string, labelName string) error {
	_, err := runBinary(kubeCtlCmd, "--namespace", namespace, "delete", resourceName, "-l", labelName)
	if err != nil {
		return errors.Wrapf(err, "deleting resource %s with label %s failed", resourceName, labelName)
	}
	return nil
}

// SecretCheckData checks the field specified in the given field
func SecretCheckData(namespace string, secretName string, fieldPath string) error {
	fetchCommand := "go-template=\"{{" + fieldPath + "}}\""
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "get", "secret", secretName, "-o", fetchCommand)
	if err != nil {
		return errors.Wrapf(err, "Getting secret %s with go template %s failed. %s", secretName, fieldPath, string(out))
	}
	if len(string(out)) > 0 {
		return nil
	}
	return nil
}

// RunCommandWithOutput runs the command specified in the container and returns output
func RunCommandWithOutput(namespace string, podName string, commandInPod string) (string, error) {
	kubectlCommand := "kubectl --namespace " + namespace + " exec -it " + podName + " " + commandInPod
	cmd := exec.Command("bash", "-c", kubectlCommand)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", errors.Wrapf(err, stderr.String())
	}
	if len(out.String()) > 0 {
		return out.String(), nil
	}
	return "", err
}

// WaitForData blocks until the specified data is available. It fails after the timeout.
func (k *Kubectl) WaitForData(namespace string, resourceName string, name string, template string, expectation string) error {
	return wait.PollImmediate(k.PollInterval, k.PollTimeout, func() (bool, error) {
		result, err := GetData(namespace, resourceName, name, template)
		if err != nil {
			return false, err
		}
		if strings.Contains(string(result), expectation) {
			return true, nil
		}
		return false, nil
	})
}

func writeTemporaryYAML(v interface{}) (string, error) {
	tmpfile, err := os.CreateTemp(os.TempDir(), "yaml-")
	if err != nil {
		return "", err
	}

	content, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	if _, err := tmpfile.Write(content); err != nil {
		return "", err
	}
	if err := tmpfile.Close(); err != nil {
		return "", err
	}

	return tmpfile.Name(), nil
}

func writeTemporaryJSON(v interface{}) (string, error) {
	tmpfile, err := os.CreateTemp(os.TempDir(), "json-")
	if err != nil {
		return "", err
	}

	content, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if _, err := tmpfile.Write(content); err != nil {
		return "", err
	}
	if err := tmpfile.Close(); err != nil {
		return "", err
	}

	return tmpfile.Name(), nil
}

func GetObject(name, namespace, resourceType string, obj interface{}) (err error) {
	r, err := GetData(namespace, resourceType, name, `jsonpath={}`)
	if err != nil {
		return err
	}

	return json.Unmarshal(r, obj)
}

// EventuallyPodMatch uses ginkgo/gomega matcher to satisfy against a namespace/label pod
func (k *Kubectl) EventuallyPodMatch(namespace, label string, timeout, poll time.Duration, mm gomega.OmegaMatcher) {
	gomega.EventuallyWithOffset(1, func() []string {
		pods, err := k.GetPodNames(namespace, label)
		if err != nil {
			fmt.Println(err)
		}
		return pods
	}, timeout, poll).Should(mm)
}

// ApplyYAML applies arbitrary interfaces with kubectl.
func (k *Kubectl) ApplyYAML(namespace string, name string, v interface{}) error {
	path, err := writeTemporaryYAML(v)
	if err != nil {
		return err
	}
	defer os.Remove(path)

	ops := []string{"apply", "-f", path}
	if namespace != "" {
		ops = append([]string{"--namespace", namespace}, ops...)
	}

	_, err = runBinary(kubeCtlCmd, ops...)
	if err != nil {
		return errors.Wrapf(err, "Applying resource %s. %s", name, v)
	}

	return nil
}

// ApplyJSON applies arbitrary interfaces with kubectl.
func (k *Kubectl) ApplyJSON(namespace string, name string, v interface{}) error {
	path, err := writeTemporaryJSON(v)
	if err != nil {
		return err
	}
	defer os.Remove(path)

	ops := []string{"apply", "-f", path}
	if namespace != "" {
		ops = append([]string{"--namespace", namespace}, ops...)
	}

	_, err = runBinary(kubeCtlCmd, ops...)
	if err != nil {
		return errors.Wrapf(err, "Applying resource %s. %s", name, v)
	}

	return nil
}

// Delete calls kubectl with the given arguments
func (k *Kubectl) Delete(args ...string) error {

	if _, err := runBinary(kubeCtlCmd, append([]string{"delete"}, args...)...); err != nil {
		return errors.Wrapf(err, "Deleting resource: %s", args)
	}

	return nil
}

// ConfigMap defines a kube ConfigMap
type ConfigMap struct {
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	Kind       string `json:"kind" yaml:"kind"`
	Metadata   struct {
		Name string `json:"name" yaml:"name"`
	} `json:"metadata" yaml:"metadata"`
	Data map[string]string `json:"data" yaml:"data"`
}

// GetConfigMap blocks until the specified data is available. It fails after the timeout.
func (k *Kubectl) GetConfigMap(namespace string, name string) (ConfigMap, error) {
	var cfgmap ConfigMap
	out, err := GetData(namespace, "configmap", name, "json")
	if err != nil {
		return cfgmap, err
	}

	err = json.Unmarshal(out, &cfgmap)
	if err != nil {
		return cfgmap, err
	}

	return cfgmap, nil
}

// GetData fetches the specified output by the given templatePath
func GetData(namespace string, resourceName string, name string, templatePath string) ([]byte, error) {
	out, err := runBinary(kubeCtlCmd, "--namespace", namespace, "get", resourceName, name, "-o", templatePath)
	msg := string(out)
	if err != nil {
		if strings.Contains(msg, "Error from server (NotFound)") {
			return []byte{}, nil
		}
		return []byte{}, errors.Wrapf(err, "getting  %s failed with template Path %s", name, templatePath)
	}
	if len(string(out)) > 0 {
		return out, nil
	}
	return []byte{}, errors.Wrapf(err, "output is empty for %s with template Path %s", name, templatePath)
}

// Run allow to control kubectl directly
func Run(s ...string) (string, error) {
	out, err := runBinary(kubeCtlCmd, s...)
	msg := string(out)
	return msg, err
}

// RunWithoutErr allow to control kubectl directly but without any StdErr
func RunWithoutErr(s ...string) (string, error) {
	out, err := runBinaryWithoutErr(kubeCtlCmd, s...)
	msg := string(out)
	return msg, err
}

// GetCRDs returns all CRDs
func GetCRDs() (*ClusterCrd, error) {
	customResource := &ClusterCrd{}
	stdOutput, err := runBinary(kubeCtlCmd, "get", "crds", "-o=json")
	if err != nil {
		return customResource, err
	}

	d := json.NewDecoder(bytes.NewReader(stdOutput))
	if err := d.Decode(customResource); err != nil {
		return customResource, err
	}

	return customResource, nil
}

// DeleteWebhooks removes existing webhookconfiguration and validatingwebhookconfiguration
func DeleteWebhooks(ns string, name string) error {
	var messages string
	webHookName := fmt.Sprintf("%s-%s", name, ns)

	_, err := runBinary(kubeCtlCmd, "delete", "--ignore-not-found", "mutatingwebhookconfiguration", webHookName)
	if err != nil {
		messages = fmt.Sprintf("%v%v\n", messages, err.Error())
	}

	_, err = runBinary(kubeCtlCmd, "delete", "--ignore-not-found", "validatingwebhookconfiguration", webHookName)
	if err != nil {
		messages = fmt.Sprintf("%v%v\n", messages, err.Error())
	}

	if messages != "" {
		return errors.New(messages)
	}
	return nil
}

// HelmBinaryVersion executes helm version and return 2 or 3
func HelmBinaryVersion() (string, error) {
	out, err := runBinary(helmCmd, "version")
	if err != nil {
		return "", err
	}

	if strings.Contains(string(out), `SemVer:"v2.`) {
		return "2", nil
	}
	if strings.Contains(string(out), `Version:"v3.`) {
		return "3", nil
	}
	return "", errors.Errorf("Failed to determine helm binary version: %s", out)
}

// RunHelmBinaryWithCustomErr executes a desire binary
func RunHelmBinaryWithCustomErr(args ...string) error {
	out, err := runBinary(helmCmd, args...)
	if err != nil {
		return &CustomError{strings.Join(append([]string{helmCmd}, args...), " "), string(out), err}
	}
	return nil
}

// RunHelmBinaryWithOutput executes a desired binary and returns the output
func RunHelmBinaryWithOutput(args ...string) (string, error) {
	out, err := runBinary(helmCmd, args...)
	if err != nil {
		return string(out), &CustomError{strings.Join(append([]string{helmCmd}, args...), " "), string(out), err}
	}
	return string(out), nil
}

// runBinary executes a binary cmd and returns the stdOutput and stdError combined
func runBinary(binaryName string, args ...string) ([]byte, error) {
	cmd := exec.Command(binaryName, args...)
	stdOutput, err := cmd.CombinedOutput()
	if err != nil {
		return stdOutput, errors.Wrapf(err, "%s cmd, failed with the following error: %s", cmd.Args, string(stdOutput))
	}
	return stdOutput, nil
}

// runBinaryWithoutErr executes a binary cmd and only returns the stdOutput
func runBinaryWithoutErr(binaryName string, args ...string) ([]byte, error) {
	cmd := exec.Command(binaryName, args...)
	stdOutput, err := cmd.Output()
	var execErr *exec.ExitError
	if errors.As(err, &execErr) {
		return stdOutput, errors.Wrapf(err, "%s cmd, failed with the following error: %s", cmd.Args, string(execErr.Stderr))
	}
	return stdOutput, nil
}

// ClusterCrd defines a list of CRDs
type ClusterCrd struct {
	Items []struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Metadata   struct {
			Name string `json:"name"`
		} `json:"metadata"`
	} `json:"items"`
}

// ContainsElement verify if a CRD exist
func (c *ClusterCrd) ContainsElement(element string) bool {
	for _, n := range c.Items {
		if n.Metadata.Name == element {
			return true
		}
	}
	return false
}

// CustomError containing stdOutput of a binary execution
type CustomError struct {
	Msg    string
	StdOut string
	Err    error
}

func (e *CustomError) Error() string {
	return fmt.Sprintf("%s:%v:%v", e.Msg, e.Err, e.StdOut)
}
