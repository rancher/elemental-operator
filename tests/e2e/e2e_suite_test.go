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

package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	checkver "github.com/hashicorp/go-version"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kubectl "github.com/rancher-sandbox/ele-testhelpers/kubectl"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	e2eConfig "github.com/rancher/elemental-operator/tests/e2e/config"
	fleetv1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	runtimeconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(clientgoscheme.Scheme))
	utilruntime.Must(managementv3.AddToScheme(clientgoscheme.Scheme))
	utilruntime.Must(elementalv1.AddToScheme(clientgoscheme.Scheme))
	utilruntime.Must(fleetv1.AddToScheme(clientgoscheme.Scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(clientgoscheme.Scheme))
}

const (
	operatorNamespace         = "cattle-elemental-system"
	operatorName              = "elemental-operator"
	operatorCRDsName          = operatorName + "-crds"
	nginxNamespace            = "ingress-nginx"
	nginxName                 = "ingress-nginx-controller"
	certManagerNamespace      = "cert-manager"
	certManagerName           = "cert-manager"
	certManagerCAInjectorName = "cert-manager-cainjector"
	cattleSystemNamespace     = "cattle-system"
	rancherName               = "rancher"
	rancherWebhook            = "rancher-webhook"
	cattleFleetNamespace      = "cattle-fleet-local-system"
	fleetAgent                = "fleet-agent"
	fleetDefaultNamespace     = "fleet-default"
	fleetNamespace            = "fleet-local"
	cattleCapiNamespace       = "cattle-provisioning-capi-system"
	capiController            = "capi-controller-manager"
	testRegistryNamespace     = "test-registry"
	testRegistryName          = "test-registry"
	sysUpgradeControllerName  = "system-upgrade-controller"
	password                  = "rancherpassword"
)

var (
	e2eCfg        *e2eConfig.E2EConfig
	cl            runtimeclient.Client
	ctx           = context.Background()
	k             = kubectl.New()
	testResources = []string{"machineregistration", "managedosversionchannel"}
	crdNames      = []string{
		"managedosimages.elemental.cattle.io",
		"machineinventories.elemental.cattle.io",
		"machineregistrations.elemental.cattle.io",
		"managedosversions.elemental.cattle.io",
		"managedosversionchannels.elemental.cattle.io",
		"machineinventoryselectors.elemental.cattle.io",
		"machineinventoryselectortemplates.elemental.cattle.io",
		"seedimages.elemental.cattle.io",
	}
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "elemental-operator e2e test Suite")
}

var _ = BeforeSuite(func() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		Fail("config path can't be empty")
	}

	var err error
	e2eCfg, err = e2eConfig.ReadE2EConfig(configPath)
	Expect(err).ToNot(HaveOccurred())

	cfg, err := runtimeconfig.GetConfig()
	Expect(err).ToNot(HaveOccurred())

	cl, err = runtimeclient.New(cfg, runtimeclient.Options{})
	Expect(err).ToNot(HaveOccurred())

	if e2eCfg.NoSetup {
		By("No setup")
		return
	}

	if isOperatorInstalled(k) { // only operator upgrade required, no furher bootstrap
		By("Upgrading the operator only", func() {
			err := kubectl.DeleteNamespace(operatorNamespace)
			Expect(err).ToNot(HaveOccurred())

			err = k.WaitForNamespaceDelete(operatorNamespace)
			Expect(err).ToNot(HaveOccurred())

			deployOperator(k, e2eCfg)

			// Somehow rancher needs to be restarted after an elemental-operator upgrade
			// to get machineregistration working
			pods, err := k.GetPodNames(cattleSystemNamespace, "")
			Expect(err).ToNot(HaveOccurred())
			for _, p := range pods {
				err = k.Delete("pod", "-n", cattleSystemNamespace, p)
				Expect(err).ToNot(HaveOccurred())
			}

			err = k.WaitForNamespaceWithPod(cattleSystemNamespace, "app=rancher")
			Expect(err).ToNot(HaveOccurred())
		})
		return
	}

	isAlreadyInstalled := func(n string) bool {
		podList := &corev1.PodList{}
		if err := cl.List(ctx, podList, &runtimeclient.ListOptions{
			Namespace: n,
		}); err != nil {
			return false
		}

		if len(podList.Items) > 0 {
			return true
		}

		return false
	}

	By("Deploying elemental-operator chart dependencies", func() {
		By("installing nginx: "+e2eCfg.NginxVersion, func() {
			if isAlreadyInstalled(nginxNamespace) {
				By("already installed")
				return
			}

			Expect(kubectl.Apply(nginxNamespace, e2eCfg.NginxURL)).To(Succeed())

			Eventually(func() bool {
				return isDeploymentReady(nginxNamespace, nginxName)
			}, 5*time.Minute, 2*time.Second).Should(BeTrue())
		})

		By("installing cert-manager: "+e2eCfg.CertManagerVersion, func() {
			if isAlreadyInstalled(certManagerNamespace) {
				By("already installed")
				return
			}

			Expect(kubectl.RunHelmBinaryWithCustomErr(
				"upgrade", "--install",
				certManagerNamespace,
				e2eCfg.CertManagerChartURL,
				"--set", "installCRDs=true",
				"--namespace", certManagerNamespace,
				"--create-namespace",
			)).To(Succeed())

			Eventually(func() bool {
				return isDeploymentReady(certManagerNamespace, certManagerName)
			}, 5*time.Minute, 2*time.Second).Should(BeTrue())

			Eventually(func() bool {
				return isDeploymentReady(certManagerNamespace, certManagerCAInjectorName)
			}, 5*time.Minute, 2*time.Second).Should(BeTrue())
		})

		By("creating cattle-system namespace", func() {
			Expect(kubectl.Apply(testRegistryNamespace, "../manifests/cattle-system-namespace.yaml")).To(Succeed())
		})

		By("installing a self-signed CA", func() {
			Expect(kubectl.Apply("cattle-system", "../manifests/test-private-ca.yaml")).To(Succeed())

			Eventually(func() bool {
				return doesSecretExist("cattle-system", "tls-ca")
			}, 5*time.Minute, 2*time.Second).Should(BeTrue())
			Eventually(func() bool {
				return doesSecretExist("cattle-system", "tls-rancher-ingress")
			}, 5*time.Minute, 2*time.Second).Should(BeTrue())

			// We need to cope with the arbitrary and hardcoded `cacerts.pem` secret key
			// See https://github.com/rancher/rancher/issues/36994

			// For this reason we fetch the cert-manager generated data['tls.crt'] from the tls-ca secret,
			// and we copy its value to  data['cacerts.pem'] where Rancher expects it.
			// See the rancher Deployment in cattle-system namespace for more info on how this is mounted.

			printCA := "-n cattle-system get secret tls-ca -o jsonpath=\"{.data['tls\\.crt']}\""
			caCert, err := kubectl.Run(strings.Split(printCA, " ")...)
			caCert = strings.ReplaceAll(caCert, `"`, "")
			Expect(err).ShouldNot(HaveOccurred())

			//patch := fmt.Sprintf(`-n cattle-system patch secret tls-ca -p "{\"data\":{\"cacerts.pem\":\"%s\"}}"`, caCert)
			//_, err = kubectl.Run(strings.Split(patch, " ")...)
			//Expect(err).ShouldNot(HaveOccurred())

			// If you wonder what the heck is happening here with the bash script, uncomment the lines above and knock yourself out.
			// It has been a long day.
			patchScript := fmt.Sprintf(`kubectl -n cattle-system patch secret tls-ca -p '{"data":{"cacerts.pem":"%s"}}'`, caCert)
			Expect(os.WriteFile("/tmp/kubectl-patch-tls-ca.sh", []byte(patchScript), os.ModePerm)).Should(Succeed())
			cmd := exec.Command("bash", "/tmp/kubectl-patch-tls-ca.sh")
			output, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Failed to patch tls-ca: %s\n", string(output))
			}
			Expect(err).ShouldNot(HaveOccurred())
		})

		By("installing rancher: "+e2eCfg.RancherVersion, func() {
			if isAlreadyInstalled(cattleSystemNamespace) {
				By("already installed")
				return
			}

			hostname := fmt.Sprintf("%s.%s", e2eCfg.ExternalIP, e2eCfg.MagicDNS)
			Expect(kubectl.RunHelmBinaryWithCustomErr(
				"upgrade", "--install",
				rancherName,
				e2eCfg.RancherChartURL,
				"--set", "replicas=1",
				"--set", "global.cattle.psp.enabled=false",
				"--set", "hostname="+hostname,
				"--set", "bootstrapPassword="+password,
				"--set", "extraEnv[0].name=CATTLE_SERVER_URL",
				"--set", "extraEnv[0].value=https://"+hostname,
				"--set", "extraEnv[1].name=CATTLE_BOOTSTRAP_PASSWORD",
				"--set", "extraEnv[1].value="+password,
				"--set", "privateCA=true",
				"--set", "agentTLSMode=system-store",
				"--namespace", cattleSystemNamespace,
			)).To(Succeed())

			Eventually(func() bool {
				return isDeploymentReady(cattleSystemNamespace, rancherName)
			}, 5*time.Minute, 2*time.Second).Should(BeTrue())

			rancherVer, err := checkver.NewVersion(e2eCfg.RancherVersion)
			Expect(err).ToNot(HaveOccurred())

			// fleet is deployed as statefulSet since 2.9
			fleetStatefulSetVer, err := checkver.NewVersion("2.9.0")
			Expect(err).ToNot(HaveOccurred())
			if rancherVer.GreaterThanOrEqual(fleetStatefulSetVer) {
				Eventually(func() bool {
					return isStatefulSetReady(cattleFleetNamespace, fleetAgent)
				}, 5*time.Minute, 2*time.Second).Should(BeTrue())
			} else {
				Eventually(func() bool {
					return isDeploymentReady(cattleFleetNamespace, fleetAgent)
				}, 5*time.Minute, 2*time.Second).Should(BeTrue())
			}

			// capi-controller exists only since Rancher Manager v2.7.8
			nonCapiVer, err := checkver.NewVersion("2.7.8")
			Expect(err).ToNot(HaveOccurred())

			if rancherVer.GreaterThanOrEqual(nonCapiVer) {
				Eventually(func() bool {
					return isDeploymentReady(cattleCapiNamespace, capiController)
				}, 5*time.Minute, 2*time.Second).Should(BeTrue())
			}

			Eventually(func() bool {
				return isDeploymentReady(cattleSystemNamespace, rancherWebhook)
			}, 5*time.Minute, 2*time.Second).Should(BeTrue())
		})

		By("installing system-upgrade-controller: "+e2eCfg.SystemUpgradeControllerVersion, func() {
			// Install CRDs first
			crdResp, err := http.Get(e2eCfg.SystemUpgradeControllerCRDsURL)
			Expect(err).ToNot(HaveOccurred())
			defer crdResp.Body.Close()
			crdData := bytes.NewBuffer([]byte{})

			_, err = io.Copy(crdData, crdResp.Body)
			Expect(err).ToNot(HaveOccurred())

			temp, err := os.CreateTemp("", "temp")
			Expect(err).ToNot(HaveOccurred())

			defer os.RemoveAll(temp.Name())
			Expect(os.WriteFile(temp.Name(), crdData.Bytes(), os.ModePerm)).To(Succeed())
			Expect(kubectl.Apply(cattleSystemNamespace, temp.Name())).To(Succeed())

			// Install system upgrade controller
			resp, err := http.Get(e2eCfg.SystemUpgradeControllerURL)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			data := bytes.NewBuffer([]byte{})

			_, err = io.Copy(data, resp.Body)
			Expect(err).ToNot(HaveOccurred())

			// It needs to look over cattle-system ns to be functional
			toApply := strings.ReplaceAll(data.String(), "namespace: system-upgrade", "namespace: "+cattleSystemNamespace)

			temp, err = os.CreateTemp("", "temp")
			Expect(err).ToNot(HaveOccurred())

			defer os.RemoveAll(temp.Name())
			Expect(os.WriteFile(temp.Name(), []byte(toApply), os.ModePerm)).To(Succeed())
			Expect(kubectl.Apply(cattleSystemNamespace, temp.Name())).To(Succeed())

			Eventually(func() bool {
				return isDeploymentReady(cattleSystemNamespace, sysUpgradeControllerName)
			}, 5*time.Minute, 2*time.Second).Should(BeTrue())
		})

		By("installing a test registry", func() {
			Expect(kubectl.Apply(testRegistryNamespace, "../manifests/test-registry.yaml")).To(Succeed())

			Eventually(func() bool {
				return isDeploymentReady(testRegistryNamespace, testRegistryName)
			}, 5*time.Minute, 2*time.Second).Should(BeTrue())
		})

	})

	deployOperator(k, e2eCfg)
})

var _ = AfterSuite(func() {
	collectArtifacts()

	// Note, this prevents concurrent tests on same cluster, but makes sure we don't leave any dangling resources from the e2e tests
	for _, r := range testResources {
		Expect(kubectl.New().Delete(r, "--all", "--all-namespaces")).To(Succeed())
	}
})

func isOperatorInstalled(k *kubectl.Kubectl) bool {
	pods, err := k.GetPodNames(operatorNamespace, "")
	Expect(err).ToNot(HaveOccurred())
	return len(pods) > 0
}

func deployOperator(k *kubectl.Kubectl, config *e2eConfig.E2EConfig) {
	By("Deploying elemental-operator chart", func() {
		Expect(kubectl.RunHelmBinaryWithCustomErr(
			"upgrade", "--install",
			operatorCRDsName,
			config.CRDsChart,
			"--set", "debug=true",
			"--set", fmt.Sprintf("replicas=%s", config.OperatorReplicas),
			"--namespace", operatorNamespace,
			"--create-namespace",
		)).To(Succeed())

		By("Waiting for CRDs to be created")
		Eventually(func() bool {
			for _, crdName := range crdNames {
				crd := &apiextensionsv1.CustomResourceDefinition{}
				if err := cl.Get(ctx,
					runtimeclient.ObjectKey{
						Name: crdName,
					},
					crd,
				); err != nil {
					return false
				}
			}
			return true
		}, 5*time.Minute, 2*time.Second).Should(BeTrue())

		Expect(kubectl.RunHelmBinaryWithCustomErr(
			"upgrade", "--install",
			operatorName,
			config.Chart,
			"--set", "debug=true",
			"--set", fmt.Sprintf("replicas=%s", config.OperatorReplicas),
			"--namespace", operatorNamespace,
			"--create-namespace",
		)).To(Succeed())

		By("Waiting for elemental-operator deployment to be available")
		Eventually(func() bool {
			return isDeploymentReady(operatorNamespace, operatorName)
		}, 5*time.Minute, 2*time.Second).Should(BeTrue())
	})
}

func isDeploymentReady(namespace, name string) bool {
	deployment := &appsv1.Deployment{}
	if err := cl.Get(ctx,
		runtimeclient.ObjectKey{
			Namespace: namespace,
			Name:      name,
		},
		deployment,
	); err != nil {
		return false
	}

	if deployment.Status.AvailableReplicas == *deployment.Spec.Replicas {
		return true
	}

	return false
}

func isStatefulSetReady(namespace, name string) bool {
	statefulSet := &appsv1.StatefulSet{}
	if err := cl.Get(ctx,
		runtimeclient.ObjectKey{
			Namespace: namespace,
			Name:      name,
		},
		statefulSet,
	); err != nil {
		return false
	}

	if statefulSet.Status.AvailableReplicas == *statefulSet.Spec.Replicas {
		return true
	}

	return false
}

func doesSecretExist(namespace, name string) bool {
	secret := &corev1.Secret{}
	if err := cl.Get(ctx,
		runtimeclient.ObjectKey{
			Namespace: namespace,
			Name:      name,
		},
		secret,
	); err != nil {
		return false
	}
	return true
}

func collectArtifacts() {
	By("Creating artifact directory")
	if _, err := os.Stat(e2eCfg.ArtifactsDir); os.IsNotExist(err) {
		Expect(os.Mkdir(e2eCfg.ArtifactsDir, os.ModePerm)).To(Succeed())
	}
	By("Getting elemental operator logs")
	getElementalOperatorLogs()
}

func getElementalOperatorLogs() {
	podList := &corev1.PodList{}
	Expect(cl.List(ctx, podList, runtimeclient.MatchingLabels{
		"app": "elemental-operator",
	},
	)).To(Succeed())

	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			for _, cStatus := range pod.Status.ContainerStatuses {
				if cStatus.Started != nil && *cStatus.Started {
					output, err := kubectl.Run("logs", pod.Name, "-c", cStatus.Name, "-n", pod.Namespace)
					Expect(err).ToNot(HaveOccurred())
					Expect(os.WriteFile(filepath.Join(e2eCfg.ArtifactsDir, pod.Name+"-"+cStatus.Name+".log"), []byte(output), 0644)).To(Succeed())
				}
			}
		}
	}
}
