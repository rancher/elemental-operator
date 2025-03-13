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

package displaycmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/klog/v2"
)

func NewDisplayCommand() *cobra.Command {
	var path string

	cmd := &cobra.Command{
		Use:   "display",
		Short: "Write the given file to standard out.",
		Run: func(_ *cobra.Command, _ []string) {
			displayRun(path)
		},
	}

	viper.AutomaticEnv()
	cmd.PersistentFlags().StringVar(&path, "file", "", "path to the file to write to standard out")
	_ = viper.BindPFlag("file", cmd.PersistentFlags().Lookup("file"))
	_ = cobra.MarkFlagRequired(cmd.PersistentFlags(), "file")

	return cmd
}

func displayRun(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		klog.Fatal("failed reading the path:", path, err)
	}

	fmt.Println(string(b))
}
