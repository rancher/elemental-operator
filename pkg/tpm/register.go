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

package tpm

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pkg/errors"
	"github.com/rancher-sandbox/go-tpm"
	"github.com/rancher/elemental-operator/pkg/dmidecode"
)

func Register(url string, caCert []byte, smbios bool, emulatedTPM bool, emulatedSeed int64, labels map[string]string) ([]byte, error) {
	var opts []tpm.Option

	if len(caCert) > 0 {
		opts = append(opts, tpm.WithCAs(caCert))
		opts = append(opts, tpm.AppendCustomCAToSystemCA)
	}

	header := http.Header{}

	if smbios {
		data, err := dmidecode.Decode()
		if err != nil {
			return nil, errors.Wrap(err, "failed to read dmidecode data")
		}

		var buf bytes.Buffer
		b64Enc := base64.NewEncoder(base64.StdEncoding, &buf)

		if err = json.NewEncoder(b64Enc).Encode(data); err != nil {
			return nil, errors.Wrap(err, "failed to encode dmidecode data")
		}

		_ = b64Enc.Close()

		chunk := make([]byte, 875) //the chunk size
		part := 1
		for {
			n, err := buf.Read(chunk)
			if err != nil {
				if err != io.EOF {
					return nil, errors.Wrap(err, "failed to read file in chunks")
				}
				break
			}
			header.Set(fmt.Sprintf("X-Cattle-Smbios-%d", part), string(chunk[0:n]))
			part++
		}
	}

	if len(labels) > 0 {
		var buf bytes.Buffer
		b64Enc := base64.NewEncoder(base64.StdEncoding, &buf)

		if err := json.NewEncoder(b64Enc).Encode(labels); err != nil {
			return nil, errors.Wrap(err, "failed to encode labels")
		}

		_ = b64Enc.Close()
		header.Set("X-Cattle-Labels", buf.String())
	}

	if emulatedTPM {
		opts = append(opts, tpm.Emulated)
		opts = append(opts, tpm.WithSeed(emulatedSeed))
	}

	opts = append(opts, tpm.WithHeader(header))

	return tpm.Get(url, opts...)
}
