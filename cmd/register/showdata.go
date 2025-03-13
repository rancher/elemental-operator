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

package main

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/rancher/elemental-operator/pkg/dmidecode"
	"github.com/rancher/elemental-operator/pkg/hostinfo"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sigs.k8s.io/yaml"
)

const (
	DUMPLEGACY        = "legacy"
	DUMPSMBIOS        = "smbios"
	DUMPHOSTINFO      = "hostinfo"
	FORMATLABELS      = "labels"
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
			"to the Elemental Operator.\nTakes the type of host data to dump " +
			"as argument, be it '" + DUMPHOSTINFO + "' (default), " + DUMPLEGACY +
			"' or '" + DUMPSMBIOS + "'.",
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{DUMPHOSTINFO, DUMPLEGACY, DUMPSMBIOS},
		RunE: func(_ *cobra.Command, args []string) error {
			return dumpdata(args, format, raw)
		},
	}

	viper.AutomaticEnv()
	cmd.Flags().BoolVarP(&raw, "raw", "r", false, "dump all collected raw data before postprocessing to refine available label templates variables")
	_ = viper.BindPFlag("raw", cmd.Flags().Lookup("raw"))
	cmd.Flags().StringVarP(&format, "format", "f", FORMATLABELS, "ouput format ['"+FORMATLABELS+"', '"+FORMATYAML+"', '"+FORMATJSON+"', '"+FORMATJSONCOMPACT+"']")
	_ = viper.BindPFlag("format", cmd.Flags().Lookup("format"))
	return cmd
}

func dumpdata(args []string, format string, raw bool) error {
	dataType := "hostinfo"
	if len(args) > 0 {
		dataType = args[0]
	}

	var mapData map[string]interface{}

	switch dataType {
	case DUMPHOSTINFO:
		hostData, err := hostinfo.Host()
		if err != nil {
			log.Fatalf("Cannot retrieve host data: %s", err)
		}

		if raw {
			mapData = hostinfo.ExtractFullData(hostData)
		} else {
			mapData = hostinfo.ExtractLabels(hostData)
		}
	case DUMPLEGACY:
		hwData, err := hostinfo.Host()
		if err != nil {
			log.Fatalf("Cannot retrieve host data: %s", err)
		}

		if raw {
			mapData = hostinfo.ExtractFullData(hwData)
		} else {
			mapData = hostinfo.ExtractLabelsLegacy(hwData)
		}
	case DUMPSMBIOS:
		smbiosData, err := dmidecode.Decode()
		if err != nil {
			log.Fatalf("Cannot retrieve SMBIOS data: %s", err)
		}

		mapData = smbiosData
	default:
		// Should never happen but manage it anyway
		log.Fatalf("Unsupported data type: %s", dataType)
	}

	var serializedData []byte
	var err error

	switch format {
	case FORMATLABELS:
		labels := map2Labels("", mapData)
		keys := make([]string, 0, len(labels))
		for k := range labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			fmt.Printf("%-52s: %q\n", k, labels[k])
		}
		return nil
	case FORMATJSON:
		serializedData, err = json.MarshalIndent(mapData, "", "  ")
	case FORMATJSONCOMPACT:
		serializedData, err = json.Marshal(mapData)
	case FORMATYAML:
		serializedData, err = yaml.Marshal(mapData)
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

func map2Labels(rootKey string, data map[string]interface{}) map[string]string {
	ret := map[string]string{}

	for key, val := range data {
		lbl := key
		if len(rootKey) > 0 {
			lbl = rootKey + "/" + lbl
		}
		if _, ok := val.(string); ok {
			lbl = "${" + lbl + "}"
			ret[lbl] = fmt.Sprintf("%s", val)
			continue
		}
		if _, ok := val.(map[string]interface{}); !ok {
			continue
		}
		for k, v := range map2Labels(lbl, val.(map[string]interface{})) {
			ret[k] = v
		}
	}
	return ret
}
