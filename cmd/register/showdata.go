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
	DUMPHW         = "hardware"
	DUMPSMBIOS     = "smbios"
	OUTJSON        = "json"
	OUTJSONCOMPACT = "json-compact"
	OUTYAML        = "yaml"
)

func newDumpDataCommand() *cobra.Command {
	var full bool
	var output string

	cmd := &cobra.Command{
		Use:     "dumpdata",
		Aliases: []string{"dump"},
		Short:   "Show host data sent during the registration phase",
		Long: "Prints to stdout the data sent by the registering client " +
			"to the Elemental Operator.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return dumpdata(output, full)
		},
	}

	viper.AutomaticEnv()
	cmd.Flags().BoolVarP(&full, "full", "f", false, "dump full, raw data before postprocessing to refine available label templates variables")
	_ = viper.BindPFlag("full", cmd.Flags().Lookup("full"))
	cmd.Flags().StringVarP(&output, "output", "o", "json", "output format ['"+OUTYAML+"', '"+OUTJSON+"', '"+OUTJSONCOMPACT+"']")
	_ = viper.BindPFlag("output", cmd.Flags().Lookup("output"))
	return cmd
}

func dumpdata(output string, full bool) error {
	var hostData interface{}

	hwData, err := hostinfo.Host()
	if err != nil {
		log.Fatalf("Cannot retrieve host data: %s", err)
	}

	if full {
		hostData, err = hostinfo.ExtractFullData(hwData)
	} else {
		hostData, err = hostinfo.ExtractLabels(hwData)
	}
	if err != nil {
		log.Fatalf("Cannot convert host data to labels: %s", err)
	}

	var serializedData []byte

	switch output {
	case OUTJSON:
		serializedData, err = json.MarshalIndent(hostData, "", "  ")
	case OUTJSONCOMPACT:
		serializedData, err = json.Marshal(hostData)
	case OUTYAML:
		serializedData, err = yaml.Marshal(hostData)
	default:
		// Should never happen but manage it anyway
		log.Fatalf("Unsupported output type: %s", output)
	}

	if err != nil {
		log.Fatalf("Cannot convert host data to %s: %s", output, err)
	}
	fmt.Printf("%s\n", string(serializedData))

	return nil
}
