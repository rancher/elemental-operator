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
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/rancher/elemental-operator/pkg/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	k3sKubeConfig         = "/etc/rancher/k3s/k3s.yaml"
	k3sKubectl            = "/usr/local/bin/kubectl"
	rkeKubeConfig         = "/etc/rancher/rke2/rke2.yaml"
	rkeKubeclt            = "/var/lib/rancher/rke2/bin/kubectl"
	elementalAgentPlanDir = "/var/lib/elemental/agent/applied/"
	rancherAgentPlanDir   = "/var/lib/rancher/agent/applied/"
	rancherAgentConf      = "/etc/rancher/agent/config.yaml"
	elementalAgentConf    = "/etc/rancher/agent/config.yaml"
	osRelease             = "/etc/os-release"
	hostnameFile          = "/etc/hostname"
	resolvConf            = "/etc/resolv.conf"
	oemDir                = "/oem/"
	systemOEMDir          = "/system/oem"
)

func getServices() []string {
	return []string{
		"elemental-system-agent",
		"rancher-system-agent",
		"k3s",
		"rke2",
		"cos-setup-boot",
		"cos-setup-fs",
		"cos-setup-initramfs",
		"cos-setup-network",
		"cos-setup-reconcile",
		"cos-setup-rootfs",
		"cos-immutable-rootfs",
		"elemental",
		"NetworkManager",
	}
}

var hostName = "unknown"
var kubectlconfig = ""
var kubectl = ""

func main() {
	cmd := &cobra.Command{
		Use:   "elemental-support",
		Short: "Gathers logs about the running system",
		Long:  "elemental-support tries to gather as much info as possible with logs about the running system for troubleshooting purposes",
		RunE: func(_ *cobra.Command, args []string) error {
			if viper.GetBool("debug") {
				logrus.SetLevel(logrus.DebugLevel)
			}
			logrus.Infof("Support version %s, commit %s, commit date %s", version.Version, version.Commit, version.CommitDate)
			return run()
		},
	}

	cmd.PersistentFlags().Bool("debug", false, "Enable debug logging")
	_ = viper.BindPFlag("debug", cmd.PersistentFlags().Lookup("debug"))

	if err := cmd.Execute(); err != nil {
		logrus.Fatalln(err)
	}
}

func run() (err error) {
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		return err
	}
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(tempDir)

	// Support version
	logrus.Info("Getting elemental-support version")
	_ = os.WriteFile(
		fmt.Sprintf("%s/elemental-support.version", tempDir),
		[]byte(fmt.Sprintf("Support version %s, commit %s, commit date %s", version.Version, version.Commit, version.CommitDate)),
		os.ModePerm)

	// copy a bit of system information
	for _, f := range []string{osRelease, resolvConf, hostnameFile} {
		logrus.Infof("Copying %s", f)
		copyFile(f, tempDir)
	}

	logrus.Infof("Copying %s", elementalAgentConf)
	copyFileWithAltName(elementalAgentConf, tempDir, "elemental-agent-config.yaml")
	logrus.Infof("Copying %s", rancherAgentConf)
	copyFileWithAltName(rancherAgentConf, tempDir, "rancher-agent-config.yaml")

	// TODO: Flag to skip certain files? They could have passwords set in them so maybe we need to search and replace
	// any sensitive fields?
	for _, f := range []string{elementalAgentPlanDir, rancherAgentPlanDir, systemOEMDir, oemDir} {
		logrus.Infof("Copying dir %s", f)
		// Full dest is the /tmp dir + the full real path of the source, so we store the paths as they are in the node
		fullDest := filepath.Join(tempDir, f)
		copyFilesInDir(f, fullDest)
	}

	for _, service := range getServices() {
		logrus.Infof("Getting service log %s", service)
		getServiceLog(service, tempDir)
	}

	// get binaries versions
	logrus.Infof("Getting elemental-cli version")
	cmd := exec.Command("elemental", "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Warnf("Could not get elemental version: %s", out)
	}
	_ = os.WriteFile(fmt.Sprintf("%s/elemental.version", tempDir), out, os.ModePerm)

	// Should probably have a version command
	logrus.Infof("Getting elemental-operator version")
	cmd = exec.Command("elemental-operator", "operator", "--namespace", "fake", "--operator-image", "whatever")
	// Don't check errors here, we expect it to fail because we pass fake stuff until we get a version command
	out, _ = cmd.CombinedOutput()
	_ = os.WriteFile(fmt.Sprintf("%s/elemental-operator.version", tempDir), out, os.ModePerm)

	// Should probably have a version command
	logrus.Infof("Getting elemental-register version")
	cmd = exec.Command("elemental-register", "/tmp/nope")
	// Don't check errors here, we expect it to fail because we pass fake stuff until we get a version command
	out, _ = cmd.CombinedOutput()
	_ = os.WriteFile(fmt.Sprintf("%s/elemental-register.version", tempDir), out, os.ModePerm)

	// Check if we have a kubeconfig before starting

	kubectlconfig, _ = getKubeConfig()
	kubectl, _ = getKubectl()

	if kubectlconfig != "" && kubectl != "" {
		// get k8s info
		for _, crd := range []string{"pods", "secrets", "nodes", "services", "deployments", "plans", "apps", "jobs"} {
			logrus.Infof("Getting k8s info for %s", crd)
			getK8sResource(crd, tempDir)
			describeK8sResource(crd, tempDir)
		}
		// get k8s logs
		for _, namespace := range []string{"cattle-system", "kube-system", "ingress-nginx", "calico-system", "cattle-fleet-system"} {
			logrus.Infof("Getting k8s logs for namespace %s", namespace)
			getK8sPodsLogs(namespace, tempDir)
		}
	} else {
		logrus.Warnf("No kubeconfig available, skipping getting k8s items")
	}

	// All done, pack it up into a nice gzip file
	hostName, err = os.Hostname()
	if err != nil {
		logrus.Warnf("Could not get hostname: %s", err)
	}

	finalFileName := fmt.Sprintf("%s-%s.tar.gz", hostName, time.Now().Format(time.RFC3339))
	finalFileName = strings.Replace(finalFileName, ":", "", -1)
	file, err := os.Create(finalFileName)
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	logrus.Infof("Creating final file")
	var buf bytes.Buffer
	err = compress(tempDir, &buf)
	if err != nil {
		logrus.Errorf("Failed to compress: %s", err)
		return err
	}
	_, err = io.Copy(file, &buf)
	if err != nil {
		logrus.Errorf("Failed to copy: %s", err)
		return err
	}

	logrus.Infof("All done. File created at %s", finalFileName)
	return nil
}

func compress(src string, buf io.Writer) error {
	zr := gzip.NewWriter(buf)
	tw := tar.NewWriter(zr)

	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	mode := fi.Mode()
	if mode.IsRegular() {
		header, err := tar.FileInfoHeader(fi, src)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		data, err := os.Open(src)
		if err != nil {
			return err
		}
		if _, err := io.Copy(tw, data); err != nil {
			return err
		}
	} else if mode.IsDir() {
		_ = filepath.Walk(src, func(file string, fi os.FileInfo, err error) error {
			// generate tar header
			var header, _ = tar.FileInfoHeader(fi, file)
			header.Name = filepath.ToSlash(file)

			// write header
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			// if not a dir, write file content
			if !fi.IsDir() {
				data, err := os.Open(file)
				if err != nil {
					return err
				}
				if _, err := io.Copy(tw, data); err != nil {
					return err
				}
			}
			return nil
		})
	} else {
		return fmt.Errorf("error: file type not supported")
	}

	// produce tar
	if err := tw.Close(); err != nil {
		return err
	}
	// produce gzip
	if err := zr.Close(); err != nil {
		return err
	}
	//
	return nil
}

func getKubeConfig() (string, error) {
	// First use it the env var if available
	kubectlconfig = os.Getenv("KUBECONFIG")
	if kubectlconfig == "" {
		if existsNoWarn(k3sKubeConfig) {
			logrus.Infof("Found k3s kubeconfig at %s", k3sKubeConfig)
			kubectlconfig = k3sKubeConfig
		}
		if existsNoWarn(rkeKubeConfig) {
			logrus.Infof("Found rke kubeconfig at %s", rkeKubeConfig)
			kubectlconfig = rkeKubeConfig
		}
		// This should not happen as far as I understand
		if existsNoWarn(k3sKubeConfig) && existsNoWarn(rkeKubeConfig) {
			return "", errors.New("both kubeconfig exists for k3s and rke2, maybe the deployment is wrong")
		}
	}
	return kubectlconfig, nil
}

func getK8sResource(resource string, dest string) {
	var err error

	cmd := exec.Command(
		kubectl,
		fmt.Sprintf("--kubeconfig=%s", kubectlconfig),
		"get",
		resource,
		"--all-namespaces",
		"-o",
		"wide",
		"--show-labels",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Warnf("Failed to get %s: %s", resource, err)
	}
	// We still want to write the output if the resource was not found or another issue occured as that can shed info on whats going on
	_ = os.WriteFile(fmt.Sprintf("%s/%s-resource.log", dest, resource), out, os.ModePerm)

}

func describeK8sResource(resource string, dest string) {
	var err error

	cmd := exec.Command(
		kubectl,
		fmt.Sprintf("--kubeconfig=%s", kubectlconfig),
		"describe",
		resource,
		"--all-namespaces",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Warnf("Failed to get %s", resource)
	}
	// We still want to write the output if the resource was not found or another issue occured as that can shed info on whats going on
	_ = os.WriteFile(fmt.Sprintf("%s/%s-describe.log", dest, resource), out, os.ModePerm)
}

// getK8sPodsLogs will get ALL the logs for ALL the pods in a given namespace
func getK8sPodsLogs(namespace, dest string) {
	var err error
	var podNames []string

	cmd := exec.Command(
		kubectl,
		fmt.Sprintf("--kubeconfig=%s", kubectlconfig),
		"get",
		"pods",
		"-n",
		namespace,
		"-o",
		"jsonpath='{.items[*].metadata.name}'",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Warnf("Failed to get pod names in namespace %s: %s", namespace, err)
	}
	cleanedOut := strings.Replace(string(out), "'", "", -1)
	if len(cleanedOut) == 0 {
		logrus.Warnf("No pods in namespace %s", namespace)
		return
	}
	podNames = strings.Split(cleanedOut, " ")

	for _, pod := range podNames {
		logrus.Debugf("Getting logs for pod %s", pod)
		if pod == "" {
			continue
		}
		cmd := exec.Command(
			kubectl,
			fmt.Sprintf("--kubeconfig=%s", kubectlconfig),
			"logs",
			pod,
			"-n",
			namespace,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			logrus.Warnf("Failed to get %s on namespace %s: %s", pod, namespace, err)
		}
		// We still want to write the output if the resource was not found or another issue occurred as that can shed info on what's going on
		_ = os.WriteFile(fmt.Sprintf("%s/%s-%s-logs.log", dest, namespace, pod), out, os.ModePerm)
	}
}

// copyFile copies a files from source to a destination path, keeping the name. Ignore any errors, we dont care
func copyFile(sourceFile string, destpath string) {
	copyFileWithAltName(sourceFile, destpath, "")
}

// copyFile copies a files from source to a destination path, altering the name. Ignore any errors, we dont care
func copyFileWithAltName(sourceFile string, destPath string, altName string) {
	if exists(sourceFile) {
		stat, err := os.Stat(sourceFile)
		if err != nil {
			logrus.Warnf("File %s does not exists", sourceFile)
			return
		}
		content, err := os.ReadFile(sourceFile)
		if err != nil {
			logrus.Warnf("Could not read file contents for %s", sourceFile)
			return
		}

		destName := fmt.Sprintf("%s/%s", destPath, stat.Name())

		if altName != "" {
			destName = fmt.Sprintf("%s/%s", destPath, altName)
		}

		err = os.WriteFile(destName, content, os.ModePerm)
		if err != nil {
			logrus.Warnf("Could not write file %s", destName)
			return
		}
	}
}

// exists checks for a path and returns a bool on its existence
func exists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		logrus.Warnf("path %s does not exist", path)
	}
	return err == nil
}

// existsNoWarn checks for a path and returns a bool on its existence. Does not raise warnings
func existsNoWarn(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func serviceExists(service string) bool {
	var out bytes.Buffer
	cmd := exec.Command("systemctl", "list-units", "--full", "-all")
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		logrus.Warn("Failed to list systemctl units")
		return false
	}
	scan := bufio.NewScanner(&out)
	for scan.Scan() {
		if strings.Contains(scan.Text(), fmt.Sprintf("%s.service", service)) {
			return true
		}
	}
	return false
}

func getServiceLog(service string, dest string) {
	var args []string
	if serviceExists(service) {
		args = append(args, "-u")
	} else {
		args = append(args, "-t")
	}
	args = append(args, service)
	args = append(args, "-o")
	args = append(args, "short-iso")
	cmd := exec.Command("journalctl", args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		_ = os.WriteFile(fmt.Sprintf("%s/%s.log", dest, service), out, os.ModePerm)
	} else {
		logrus.Warningf("Failed to get log for service %s", service)
	}

}

// redactPasswords removes any occurrences of a passwd or password set in yaml/cloud-config files usually
func redactPasswords(input []byte) []byte {
	passwd := regexp.MustCompile(`(passwd)([\s=\t\:]{1,3})(.*)`)
	password := regexp.MustCompile(`(password)([\s=\t\:]{1,3})(.*)`)
	redacted := passwd.ReplaceAll(input, []byte("$1$2*****"))
	redacted = password.ReplaceAll(redacted, []byte("$1$2*****"))
	return redacted
}

// copyFilesInDir copies all files in the source dir to destination path, keeping the name. Follows dirs inside dirs.
func copyFilesInDir(src, dest string) {
	info, err := os.Lstat(src)
	if err != nil {
		logrus.Warnf("Error opening %s", src)
		return
	}
	switch {
	case info.IsDir():
		copyDir(src, dest)
	default:
		copySingleFile(src, dest)
	}
}

func copySingleFile(src string, dest string) {
	logrus.Debugf("Copying %s into %s", src, dest)
	original, err := os.ReadFile(src)
	if err != nil {
		logrus.Warnf("Failed to read: %s", err)
		return
	}
	redacted := redactPasswords(original)
	err = os.MkdirAll(filepath.Dir(dest), os.ModeDir|os.ModePerm)
	if err != nil {
		logrus.Warnf("Failed to create dir: %s", err)
		return
	}
	err = os.WriteFile(dest, redacted, os.ModePerm)
	if err != nil {
		logrus.Warnf("Failed to write: %s", err)
		return
	}
}

func copyDir(srcdir string, destdir string) {
	contents, err := os.ReadDir(srcdir)
	if err != nil {
		return
	}
	for _, content := range contents {
		cs, cd := filepath.Join(srcdir, content.Name()), filepath.Join(destdir, content.Name())
		copyFilesInDir(cs, cd)
	}
}

func getKubectl() (string, error) {
	if existsNoWarn(k3sKubectl) {
		logrus.Infof("Found k3s kubectl at %s", k3sKubectl)
		return k3sKubectl, nil
	}
	if existsNoWarn(rkeKubeclt) {
		logrus.Infof("Found rke kubectl at %s", rkeKubeclt)
		return rkeKubeclt, nil
	}

	return "", errors.New("Cant find kubectl")
}
