/*
Copyright © 2022 - 2026 SUSE LLC

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

package downloadcmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/klog/v2"
)

const defaultTimeout = "30s"

func NewDownloadCommand() *cobra.Command {
	var path, url string

	cmd := &cobra.Command{
		Use:   "download",
		Short: "Downloads the given url to the given file",
		Run: func(_ *cobra.Command, _ []string) {
			downloadRun(url, path)
		},
	}

	viper.AutomaticEnv()
	cmd.PersistentFlags().StringVar(&path, "file", "", "path to the file to write downloaded data")
	_ = viper.BindPFlag("file", cmd.PersistentFlags().Lookup("file"))
	_ = cobra.MarkFlagRequired(cmd.PersistentFlags(), "file")
	cmd.PersistentFlags().StringVar(&url, "url", "", "URL to download data from")
	_ = viper.BindPFlag("url", cmd.PersistentFlags().Lookup("url"))
	_ = cobra.MarkFlagRequired(cmd.PersistentFlags(), "url")

	return cmd
}

func downloadRun(url, path string) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	go func() {
		<-sigc
		os.Exit(1)
	}()

	timeoutEnv := os.Getenv(elementalv1.TimeoutEnvVar)
	if timeoutEnv == "" {
		timeoutEnv = defaultTimeout
	}

	timeout, err := time.ParseDuration(timeoutEnv)
	if err != nil {
		klog.Fatal(fmt.Errorf("failed to parse timeout: %w", err))
	}

	client := &http.Client{
		Timeout: timeout,
	}

	out, err := os.Create(path)
	if err != nil {
		klog.Fatal(fmt.Errorf("failed to create %s: %w", path, err))
	}
	defer out.Close()

	resp, err := client.Get(url)
	if err != nil {
		klog.Fatal(fmt.Errorf("failed to get %s: %w", url, err))
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		klog.Fatal(fmt.Errorf("failed reading response body: %w", err))
	}
}
