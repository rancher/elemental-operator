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

	"github.com/rancher/elemental-operator/pkg/dmidecode"
	"github.com/rancher/elemental-operator/pkg/hostinfo"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	DUMPHW     = "hardware"
	DUMPSMBIOS = "smbios"
)

func newDumpDataCommand() *cobra.Command {
	var raw bool

	cmd := &cobra.Command{
		Use:     "dumpdata",
		Aliases: []string{"dump"},
		Short:   "Show host data sent during the registration phase",
		Long: "Prints to stdout the data sent by the registering client " +
			"to the Elemental Operator.\nTakes the type of host data to dump " +
			"as argument, be it '" + DUMPHW + "' or '" + DUMPSMBIOS + "'.",
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{DUMPHW, DUMPSMBIOS},
		RunE: func(_ *cobra.Command, args []string) error {
			return dumpdata(args, raw)
		},
	}

	viper.AutomaticEnv()
	cmd.Flags().BoolVarP(&raw, "raw", "r", false, "dump raw data before conversion to label templates' variables")
	_ = viper.BindPFlag("raw", cmd.Flags().Lookup("raw"))

	return cmd
}

func dumpdata(args []string, raw bool) error {
	dataType := "hardware"
	if len(args) > 0 {
		dataType = args[0]
	}

	var hostData interface{}

	switch dataType {
	case DUMPHW:
		hwData, err := hostinfo.Host()
		if err != nil {
			log.Fatalf("Cannot retrieve host data: %s", err)
		}

		if raw {
			hostData = hwData
		} else {
			dataMap, err := hostinfo.ExtractLabels(hwData)
			if err != nil {
				log.Fatalf("Cannot convert host data to labels: %s", err)
			}
			hostData = dataMap
		}

	case DUMPSMBIOS:
		smbiosData, err := dmidecode.Decode()
		if err != nil {
			log.Fatalf("Cannot retrieve SMBIOS data: %s", err)
		}

		hostData = smbiosData

	default:
		// Should never happen but manage it anyway
		log.Fatalf("Unsupported data type: %s", dataType)
	}

	jsonData, err := json.MarshalIndent(hostData, "", "  ")
	if err != nil {
		log.Fatalf("Cannot convert host data to json: %s", err)
	}
	fmt.Printf("%s\n", string(jsonData))

	return nil
}
