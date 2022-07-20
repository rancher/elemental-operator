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

package config_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"time"

	gotpm "github.com/rancher-sandbox/go-tpm"

	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rancher/elemental-operator/pkg/config"
)

func writeRead(conn *websocket.Conn, input []byte) ([]byte, error) {
	writer, err := conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return nil, err
	}

	if _, err := writer.Write(input); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	_, reader, err := conn.NextReader()
	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(reader)
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Mimics a WS server which accepts TPM Bearer token
func WSServer(ctx context.Context, data map[string]interface{}) {
	s := http.Server{
		Addr:         "127.0.0.1:9980",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	m := http.NewServeMux()
	m.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		conn, _ := upgrader.Upgrade(w, r, nil) // error ignored for sake of simplicity

		for {

			token := r.Header.Get("Authorization")
			ek, at, err := gotpm.GetAttestationData(token)
			if err != nil {
				fmt.Println("error", err.Error())
				return
			}

			secret, challenge, err := gotpm.GenerateChallenge(ek, at)
			if err != nil {
				fmt.Println("error", err.Error())
				return
			}

			resp, _ := writeRead(conn, challenge)

			if err := gotpm.ValidateChallenge(secret, resp); err != nil {
				fmt.Println(string(resp))
				fmt.Println("error validating challenge", err.Error())
				return
			}

			writer, _ := conn.NextWriter(websocket.BinaryMessage)
			_ = json.NewEncoder(writer).Encode(data)
		}
	})

	s.Handler = m

	go s.ListenAndServe()
	go func() {
		<-ctx.Done()
		_ = s.Shutdown(ctx)
	}()
}

var _ = Describe("os2 config unit tests", func() {

	Context("convert to environment configuration", func() {
		var c Config
		It("handle empty config", func() {
			c = Config{}
			e, err := ToEnv(c.Elemental.Install)
			Expect(err).ToNot(HaveOccurred())
			Expect(e).To(BeEmpty())
		})
		It("converts to env slice installation parameters", func() {
			c = Config{
				CloudConfig: map[string]interface{}{
					"random": "data",
				},
				Elemental: Elemental{
					// Those settings below are tied to the
					// elemental installer.
					Install: Install{
						Device:     "foob",
						ConfigURLs: []string{"fooc", "food"},
						Firmware:   "efi",
						ISO:        "http://foo.bar",
						NoFormat:   true,
						Debug:      true,
						PowerOff:   true,
						Reboot:     true,
						TTY:        "foo",
						SystemURI:  "docker:container",
					},
				},
			}
			e, err := ToEnv(c.Elemental.Install)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(e)).To(Equal(10))
			Expect(e).To(
				ContainElements(
					"ELEMENTAL_INSTALL_TARGET=foob",
					"ELEMENTAL_INSTALL_CLOUD_INIT=fooc,food",
					"ELEMENTAL_INSTALL_FIRMWARE=efi",
					"ELEMENTAL_INSTALL_ISO=http://foo.bar",
					"ELEMENTAL_INSTALL_NO_FORMAT=true",
					"ELEMENTAL_DEBUG=true",
					"ELEMENTAL_POWEROFF=true",
					"ELEMENTAL_REBOOT=true",
					"ELEMENTAL_INSTALL_TTY=foo",
					"ELEMENTAL_INSTALL_SYSTEM=docker:container",
				),
			)
		})
	})
})
