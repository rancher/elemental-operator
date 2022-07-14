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

	/*var c Config
	var data map[string]interface{}

	BeforeEach(func() {
		c = Config{}
		data = map[string]interface{}{
			"elemental": map[string]interface{}{
				"install": map[string]string{
					"iso": "foo",
				},
			},
		}
	})*/

	/*Context("Validation", func() {
			It("fails if iso and system-uri are both used at the same time", func() {
				f, err := ioutil.TempFile("", "xxxxtest")
				Expect(err).ToNot(HaveOccurred())
				defer os.Remove(f.Name())

				_ = ioutil.WriteFile(f.Name(), []byte(`
	elemental:
	  install:
	    system-uri: "docker/image:test"
	    iso: "test"
	`), os.ModePerm)

				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer cancel()
				_, err = ReadConfig(ctx, f.Name(), false)
				Expect(err).To(HaveOccurred())
			})
			It("fails if iso and system-uri are both empty", Serial, func() {
				f, err := ioutil.TempFile("", "xxxxtest")
				Expect(err).ToNot(HaveOccurred())
				defer os.Remove(f.Name())

				_ = ioutil.WriteFile(f.Name(), []byte(`
	elemental:
	  registration:
	    emulateTPM: true
	    noSMBIOS: true
	    emulatedTPMSeed: "5"
	  install:
	    firmware: efi
	`), os.ModePerm)

				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer cancel()
				// Empty the install key so there is no isourl nor containerImage
				values.PutValue(data, "", "elemental", "install")
				WSServer(ctx, data)
				_, err = ReadConfig(ctx, f.Name(), false)
				Expect(err).To(HaveOccurred())
			})
		})*/

	Context("convert to environment configuration", func() {
		var c Config
		It("handle empty config", func() {
			c = Config{}
			e, err := ToEnv(c)
			Expect(err).ToNot(HaveOccurred())
			Expect(e).To(BeEmpty())
		})
		It("converts to env slice installation parameters", func() {
			c = Config{
				Data: map[string]interface{}{
					"random": "data",
				},
				Elemental: Elemental{
					// Those settings below are tied to the
					// elemental installer.
					Install: Install{
						Device:    "foob",
						ConfigURL: "fooc",
						Firmware:  "efi",
						ISO:       "http://foo.bar",
						NoFormat:  true,
						Debug:     true,
						PowerOff:  true,
						TTY:       "foo",
						SystemURI: "docker:container",
						SSHKeys:   []string{"github:mudler"},
					},
				},
			}
			e, err := ToEnv(c)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(e)).To(Equal(10))
			Expect(e).To(
				ContainElements(
					"ELEMENTAL_INSTALL_SSH_KEYS=[github:mudler]",
					"ELEMENTAL_INSTALL_TARGET=foob",
					"ELEMENTAL_INSTALL_CLOUD_INIT=fooc",
					"ELEMENTAL_INSTALL_FIRMWARE=efi",
					"ELEMENTAL_INSTALL_ISO=http://foo.bar",
					"ELEMENTAL_INSTALL_NO_FORMAT=true",
					"ELEMENTAL_DEBUG=true",
					"ELEMENTAL_POWEROFF=true",
					"ELEMENTAL_INSTALL_TTY=foo",
					"ELEMENTAL_INSTALL_SYSTEM=docker:container",
				),
			)
		})
	})

	/*Context("reading config file", func() {
			It("reads iso_url and registrationUrl", func() {
				f, err := ioutil.TempFile("", "xxxxtest")
				Expect(err).ToNot(HaveOccurred())
				defer os.Remove(f.Name())

				_ = ioutil.WriteFile(f.Name(), []byte(`
	elemental:
	  registration:
	    url: foobaz
	  install:
	    iso: "foo_bar"
	`), os.ModePerm)

				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer cancel()
				c, err := ReadConfig(ctx, f.Name(), false)
				Expect(err).ToNot(HaveOccurred())
				Expect(c.Elemental.Registration.URL).To(Equal("foobaz"))
				Expect(c.Elemental.Install.ISO).To(Equal("foo_bar"))
			})
			It("reads iso_url only, without contacting a registrationUrl server", func() {
				f, err := ioutil.TempFile("", "xxxxtest")
				Expect(err).ToNot(HaveOccurred())
				defer os.Remove(f.Name())

				_ = ioutil.WriteFile(f.Name(), []byte(`
	elemental:
	  install:
	    iso: "foo_bar"
	`), os.ModePerm)

				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer cancel()
				c, err := ReadConfig(ctx, f.Name(), false)
				Expect(err).ToNot(HaveOccurred())
				Expect(c.Elemental.Install.ISO).To(Equal("foo_bar"))
			})
			It("reads containerImage, without contacting a registrationUrl server", func() {
				f, err := ioutil.TempFile("", "xxxxtest")
				Expect(err).ToNot(HaveOccurred())
				defer os.Remove(f.Name())

				_ = ioutil.WriteFile(f.Name(), []byte(`
	elemental:
	  install:
	    system-uri: "docker:docker/image:test"
	`), os.ModePerm)

				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer cancel()
				c, err := ReadConfig(ctx, f.Name(), false)
				Expect(err).ToNot(HaveOccurred())
				Expect(c.Elemental.Install.ISO).To(Equal(""))
				Expect(c.Elemental.Install.SystemURI).To(Equal("docker:docker/image:test"))
			})
			It("reads system-uri and registration url", func() {

				f, err := ioutil.TempFile("", "xxxxtest")
				Expect(err).ToNot(HaveOccurred())
				defer os.Remove(f.Name())

				_ = ioutil.WriteFile(f.Name(), []byte(`
	elemental:
	  registration:
	    url: "foobar"
	  install:
	    system-uri: "docker:docker/image:test"
	`), os.ModePerm)

				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer cancel()
				c, err := ReadConfig(ctx, f.Name(), false)
				Expect(err).ToNot(HaveOccurred())
				Expect(c.Elemental.Install.SystemURI).To(Equal("docker:docker/image:test"))
			})

			It("reads iso", func() {
				f, err := ioutil.TempFile("", "xxxxtest")
				Expect(err).ToNot(HaveOccurred())
				defer os.Remove(f.Name())

				_ = ioutil.WriteFile(f.Name(), []byte(`
	elemental:
	  install:
	    iso: "foo_bar"
	`), os.ModePerm)

				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer cancel()
				c, err := ReadConfig(ctx, f.Name(), false)
				Expect(err).ToNot(HaveOccurred())
				Expect(c.Elemental.Install.ISO).To(Equal("foo_bar"))
			})

			It("reads install ssh-keys", Label("format"), func() {
				f, err := ioutil.TempFile("", "xxxxtest")
				Expect(err).ToNot(HaveOccurred())
				defer os.Remove(f.Name())

				_ = ioutil.WriteFile(f.Name(), []byte(`
	elemental:
	  install:
	    ssh-keys:
	    - foo
	`), os.ModePerm)

				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer cancel()
				c, err := ReadConfig(ctx, f.Name(), false)
				Expect(err).ToNot(HaveOccurred())
				Expect(c.Elemental.Install.SSHKeys).To(Equal([]string{"foo"}))
			})
		})

		Context("writing config", func() {
			It("uses cloud-init format, but if data is present, takes over", func() {
				c = Config{
					Data: map[string]interface{}{
						"users": []struct {
							User string `json:"user"`
							Pass string `json:"pass"`
						}{{"foo", "Bar"}},
					},

					Elemental: Elemental{
						Install: Install{
							Automatic: true,
							Firmware:  "efi",
							SSHKeys:   []string{"github:mudler"},
							ISO:       "http://foo.bar",
						}, Registration: Registration{
							URL: "Foo",
						},
					},
				}

				f, err := ioutil.TempFile("", "xxxxtest")
				Expect(err).ToNot(HaveOccurred())
				defer os.Remove(f.Name())

				err = ToFile(c, f.Name())
				Expect(err).ToNot(HaveOccurred())

				ff, _ := ioutil.ReadFile(f.Name())
				Expect(string(ff)).To(Equal("#cloud-config\nusers:\n- pass: Bar\n  user: foo\n"))
			})
			It("writes cloud-init files", func() {
				c = Config{
					Elemental: Elemental{
						Install: Install{
							Automatic: true,
							Firmware:  "efi",
							SSHKeys:   []string{"github:mudler"},
							ISO:       "http://foo.bar",
						}, Registration: Registration{
							URL: "Foo",
						},
					},
				}

				f, err := ioutil.TempFile("", "xxxxtest")
				Expect(err).ToNot(HaveOccurred())
				defer os.Remove(f.Name())

				err = ToFile(c, f.Name())
				Expect(err).ToNot(HaveOccurred())

				ff, _ := ioutil.ReadFile(f.Name())
				Expect(string(ff)).To(Equal("#cloud-config\nelemental: {}\nssh_authorized_keys:\n- github:mudler\n"))
			})
			It("reads iso_url by contacting a registrationUrl server", Serial, func() {

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				WSServer(ctx, data)
				f, err := ioutil.TempFile("", "xxxxtest")
				Expect(err).ToNot(HaveOccurred())
				defer os.Remove(f.Name())

				_ = ioutil.WriteFile(f.Name(), []byte(`
	elemental:
	  registration:
	    emulateTPM: true
	    noSMBIOS: true
	    emulatedTPMSeed: "5"
	    url: "http://127.0.0.1:9980/test"
	`), os.ModePerm)

				c, err := ReadConfig(ctx, f.Name(), false)
				Expect(err).ToNot(HaveOccurred())
				Expect(c.Elemental.Install.ISO).To(Equal("foo"))

				_ = ioutil.WriteFile(f.Name(), []byte(`
	elemental:
	  registration:
	    emulateTPM: true
	    noSMBIOS: true
	    emulatedTPMSeed: "5"
	    url: "http://127.0.0.1:9980/test"
	`), os.ModePerm)

				c, err = ReadConfig(ctx, f.Name(), false)
				Expect(err).ToNot(HaveOccurred())
				Expect(c.Elemental.Install.ISO).To(Equal("foo"))
			})
			It("reads system-uri by contacting a registrationUrl server", Serial, func() {

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				// Override the install value on the data
				value := map[string]string{"system-uri": "docker:test"}
				values.PutValue(data, value, "elemental", "install")

				WSServer(ctx, data)
				f, err := ioutil.TempFile("", "xxxxtest")
				Expect(err).ToNot(HaveOccurred())
				defer os.Remove(f.Name())

				_ = ioutil.WriteFile(f.Name(), []byte(`
	elemental:
	  registration:
	    emulateTPM: true
	    noSMBIOS: true
	    emulatedTPMSeed: "5"
	    url: "http://127.0.0.1:9980/test"
	`), os.ModePerm)

				c, err := ReadConfig(ctx, f.Name(), false)
				Expect(err).ToNot(HaveOccurred())
				Expect(c.Elemental.Install.SystemURI).To(Equal("docker:test"))

				_ = ioutil.WriteFile(f.Name(), []byte(`
	elemental:
	  registration:
	    emulateTPM: true
	    noSMBIOS: true
	    url: "http://127.0.0.1:9980/test"
	`), os.ModePerm)

				c, err = ReadConfig(ctx, f.Name(), false)
				Expect(err).ToNot(HaveOccurred())
				Expect(c.Elemental.Install.SystemURI).To(Equal("docker:test"))
			})
			It("doesn't error out if isoUrl or containerImage are not provided", Serial, func() {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				// Override the install value on the data
				value := map[string]string{}
				values.PutValue(data, value, "elemental", "install")

				WSServer(ctx, data)
				f, err := ioutil.TempFile("", "xxxxtest")
				Expect(err).ToNot(HaveOccurred())
				defer os.Remove(f.Name())

				_ = ioutil.WriteFile(f.Name(), []byte(`
	elemental:
	  registration:
	    emulateTPM: true
	    noSMBIOS: true
	    url: "http://127.0.0.1:9980/test"
	`), os.ModePerm)

				c, err := ReadConfig(ctx, f.Name(), false)
				Expect(err).ToNot(HaveOccurred())
				Expect(c.Elemental.Install.SystemURI).To(Equal(""))
				Expect(c.Elemental.Install.ISO).To(Equal(""))
			})
		})*/
})
