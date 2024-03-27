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

package register

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/plainauth"
	"github.com/rancher/elemental-operator/pkg/tpm"
)

func echo(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		err = conn.WriteMessage(msgType, data)
		if err != nil {
			return
		}
	}
}

var _ = Describe("authenticate", Label("registration", "core"), func() {
	var server *httptest.Server
	var conn *websocket.Conn
	var registration elementalv1.Registration

	BeforeEach(func() {
		server = httptest.NewServer(http.HandlerFunc(echo))
		DeferCleanup(server.Close)
	})

	Context("plain authentication data is provided", func() {
		var auth authClient

		BeforeEach(func() {
			registration = elementalv1.Registration{
				Auth: "mac",
			}
			auth = &plainauth.AuthClient{}
			err := auth.Init(registration)
			Expect(err).ToNot(HaveOccurred())

			url := "ws" + strings.TrimPrefix(server.URL, "http")
			conn, _, err = websocket.DefaultDialer.Dial(url, nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("succedes if the remote endpoint acknowledges authentication", func() {
			err := conn.WriteMessage(websocket.BinaryMessage, insertMessageType(MsgReady, []byte("data")))
			Expect(err).ToNot(HaveOccurred())

			err = authenticate(conn, auth)
			Expect(err).ToNot(HaveOccurred())
		})

		It("fails if the remote endpoint does not acknowledge authentication", func() {
			err := conn.WriteMessage(websocket.BinaryMessage, insertMessageType(MsgError, []byte("data")))
			Expect(err).ToNot(HaveOccurred())

			err = authenticate(conn, auth)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("expecting '%v' but got", MsgReady)))
		})
	})
})

var _ = Describe("initialize websocket", Label("registration", "core"), func() {
	var server *httptest.Server
	var auth authClient

	BeforeEach(func() {
		server = httptest.NewServer((http.HandlerFunc(echo)))
		DeferCleanup(server.Close)
	})

	When("authentication data is provided", func() {
		var registration elementalv1.Registration

		BeforeEach(func() {
			registration = elementalv1.Registration{
				EmulateTPM:      true,
				EmulatedTPMSeed: 10,
			}
			auth = &tpm.AuthClient{}

			err := auth.Init(registration)
			Expect(err).ToNot(HaveOccurred())
		})

		It("establishes a working connection with the remote endpoint", func() {
			conn, err := initWebsocketConn(server.URL+"/token", []byte{}, auth)
			Expect(err).ToNot(HaveOccurred())

			err = conn.WriteMessage(websocket.BinaryMessage, []byte("test"))
			Expect(err).ToNot(HaveOccurred())

			_, data, err := conn.ReadMessage()
			Expect(err).ToNot(HaveOccurred())
			Expect(string(data)).To(Equal("test"))
		})

		Context("if registration URL is missing", func() {
			It("returns a dialer error", func() {
				_, err := initWebsocketConn("", []byte{}, auth)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("malformed ws or wss URL"))
			})
		})
	})

	When("authentication data is not provided", func() {

		BeforeEach(func() {
			auth = &plainauth.AuthClient{}
		})

		It("returns an authentication error", func() {
			_, err := initWebsocketConn(server.URL+"/token", []byte{}, auth)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot generate authentication"))
		})
	})
})

var _ = Describe("authenticator", Label("registration", "core"), func() {
	var reg elementalv1.Registration
	var state *State

	BeforeEach(func() {
		state = &State{}
	})

	It("returns TPM authentication on auth=tpm registration", func() {
		reg = elementalv1.Registration{
			Auth: "tpm",
		}

		auth, err := getAuthenticator(reg, state)
		Expect(err).ToNot(HaveOccurred())
		Expect(auth.GetName()).To(Equal("TPM"))
		Expect(state.EmulatedTPM).To(BeFalse())
	})

	It("fills State with TPM emulation data from registration", func() {
		reg = elementalv1.Registration{
			Auth:            "tpm",
			EmulateTPM:      true,
			EmulatedTPMSeed: 10,
		}

		auth, err := getAuthenticator(reg, state)
		Expect(err).ToNot(HaveOccurred())
		Expect(auth.GetName()).To(Equal("TPM"))
		Expect(state.EmulatedTPM).To(BeTrue())
		Expect(state.EmulatedTPMSeed).To(Equal(int64(10)))
	})

	It("returns plain authentication on auth=mac registration", func() {
		reg = elementalv1.Registration{
			Auth: "mac",
		}

		auth, err := getAuthenticator(reg, state)
		Expect(err).ToNot(HaveOccurred())
		Expect(auth.GetName()).To(Equal("Plain"))
	})

	It("returns plain authentication on auth=sys-uuid registration", func() {
		reg = elementalv1.Registration{
			Auth: "sys-uuid",
		}

		auth, err := getAuthenticator(reg, state)
		Expect(err).ToNot(HaveOccurred())
		Expect(auth.GetName()).To(Equal("Plain"))
	})

	It("returns error on unknown authentication", func() {
		reg = elementalv1.Registration{
			Auth: "unknown",
		}

		_, err := getAuthenticator(reg, state)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported authentication"))
	})
})
