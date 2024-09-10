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

package main

import (
	"encoding/json"
	"fmt"

	"github.com/rancher/elemental-operator/pkg/hostinfo"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sigs.k8s.io/yaml"
)

const (
	DUMPHW            = "hardware"
	DUMPSMBIOS        = "smbios"
	FORMATJSON        = "json"
	FORMATJSONCOMPACT = "json-compact"
	FORMATYAML        = "yaml"
)

func newDumpDataCommand() *cobra.Command {
	var raw bool
	var format string

	cmd := &cobra.Command{
		Use:     "dumpdata",
		Aliases: []string{"dump"},
		Short:   "Show host data sent during the registration phase",
		Long: "Prints to stdout the data sent by the registering client " +
			"to the Elemental Operator.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return dumpdata(format, raw)
		},
	}

	viper.AutomaticEnv()
	cmd.Flags().BoolVarP(&raw, "raw", "r", false, "dump all collected raw data before postprocessing to refine available label templates variables")
	_ = viper.BindPFlag("raw", cmd.Flags().Lookup("raw"))
	cmd.Flags().StringVarP(&format, "format", "f", "json", "ouput format ['"+FORMATYAML+"', '"+FORMATJSON+"', '"+FORMATJSONCOMPACT+"']")
	_ = viper.BindPFlag("format", cmd.Flags().Lookup("format"))
	return cmd
}

func dumpdata(format string, raw bool) error {
	var hostData map[string]interface{}

	hwData, err := hostinfo.Host()
	if err != nil {
		log.Fatalf("Cannot retrieve host data: %s", err)
	}

	if raw {
		hostData = hostinfo.ExtractFullData(hwData)
	} else {
		hostData = hostinfo.ExtractLabels(hwData)
	}
	if err != nil {
		log.Fatalf("Cannot convert host data to labels: %s", err)
	}

	var serializedData []byte

	switch format {
	case FORMATJSON:
		serializedData, err = json.MarshalIndent(hostData, "", "  ")
	case FORMATJSONCOMPACT:
		serializedData, err = json.Marshal(hostData)
	case FORMATYAML:
		serializedData, err = yaml.Marshal(hostData)
	default:
		// Should never happen but manage it anyway
		log.Fatalf("Unsupported output type: %s", format)
	}

	if err != nil {
		log.Fatalf("Cannot convert host data to %s: %s", format, err)
	}
	fmt.Printf("%s\n", string(serializedData))

	return nil
}
