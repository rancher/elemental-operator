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
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/twpayne/go-vfs"
	"github.com/twpayne/go-vfs/vfst"
	"gopkg.in/yaml.v3"
)

func TestRegister(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Register State Suite")
}

var (
	testStateDir  = "/test/register/state"
	testStatePath = fmt.Sprintf("%s/%s", testStateDir, "state.yaml")
	loc, _        = time.LoadLocation("Europe/Berlin")
	stateFixture  = State{
		InitialRegistration: time.Now().UTC(),
		LastUpdate:          time.Now().UTC(),
		EmulatedTPM:         true,
		EmulatedTPMSeed:     123456789,
	}
)

var _ = Describe("is state updatable", Label("registration", "state"), func() {
	It("returns false if the state is new", func() {
		state := State{}
		Expect(state.IsUpdatable()).To(BeFalse())
	})
	It("returns true if the initial registration already happened", func() {
		state := State{
			InitialRegistration: time.Now(),
		}
		Expect(state.IsUpdatable()).To(BeTrue())
	})
})

var _ = Describe("has state update elapsed", Label("registration", "state"), func() {
	It("returns false if the state is new", func() {
		state := State{}
		Expect(state.HasLastUpdateElapsed(-1 * time.Hour)).To(BeTrue())
	})
	It("returns true last update time is more than suppress timer ago", func() {
		state := State{
			LastUpdate: time.Now().Add(-10 * time.Hour),
		}
		Expect(state.HasLastUpdateElapsed(1 * time.Hour)).To(BeTrue())
	})
})

var _ = Describe("init file state handler", Label("registration", "state"), func() {
	var fs vfs.FS
	var handler StateHandler
	var err error
	var fsCleanup func()
	BeforeEach(func() {
		fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{})
		Expect(err).ToNot(HaveOccurred())
		handler = NewFileStateHandler(fs)
		DeferCleanup(fsCleanup)
	})
	When("directory exists", func() {
		BeforeEach(func() {
			Expect(vfs.MkdirAll(fs, testStateDir, os.ModePerm)).ToNot(HaveOccurred())
		})
		It("should create empty state", func() {
			Expect(handler.Init(testStatePath)).ToNot(HaveOccurred())
			Expect(fs.ReadFile(testStatePath)).To(Equal([]byte("{}\n")))
		})
		It("should do nothing if file exists", func() {
			testBytes := []byte("foobar")
			Expect(fs.WriteFile(testStatePath, testBytes, 0700)).ToNot(HaveOccurred())
			Expect(handler.Init(testStatePath)).ToNot(HaveOccurred())
			Expect(fs.ReadFile(testStatePath)).To(Equal(testBytes))
		})
	})
	When("directory does not exist", func() {
		It("should create empty state", func() {
			Expect(handler.Init(testStatePath)).ToNot(HaveOccurred())
			Expect(fs.ReadFile(testStatePath)).To(Equal([]byte("{}\n")))
		})
	})
})

var _ = Describe("load state from filesystem", Label("registration", "state"), func() {
	var fs vfs.FS
	var handler StateHandler
	var err error
	var fsCleanup func()
	BeforeEach(func() {
		fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{})
		Expect(err).ToNot(HaveOccurred())
		handler = NewFileStateHandler(fs)
		DeferCleanup(fsCleanup)
	})
	When("init handler", func() {
		BeforeEach(func() {
			Expect(handler.Init(testStatePath)).ToNot(HaveOccurred())
		})
		It("should return state if state is deserializable", func() {
			bytes, err := yaml.Marshal(stateFixture)
			Expect(err).ToNot(HaveOccurred())
			Expect(fs.WriteFile(testStatePath, bytes, 0700)).ToNot(HaveOccurred())
			Expect(handler.Load()).To(Equal(stateFixture))
		})
		It("should return error if state is not deserializable", func() {
			bytes := []byte("I am definitely not yaml")
			Expect(fs.WriteFile(testStatePath, bytes, 0700)).ToNot(HaveOccurred())
			state, err := handler.Load()
			Expect(state).To(Equal(State{}))
			Expect(err).To(MatchError(errDecodingState))
		})
	})
	When("missing init handler", func() {
		It("should return error", func() {
			_, err := handler.Load()
			Expect(err).To(HaveOccurred())
		})
	})
})

var _ = Describe("save state to filesystem", Label("registration", "state"), func() {
	var fs vfs.FS
	var handler StateHandler
	var err error
	var fsCleanup func()
	BeforeEach(func() {
		fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{})
		Expect(err).ToNot(HaveOccurred())
		handler = NewFileStateHandler(fs)
		DeferCleanup(fsCleanup)
	})
	When("init handler", func() {
		BeforeEach(func() {
			Expect(handler.Init(testStatePath)).ToNot(HaveOccurred())
		})
		It("should return no error", func() {
			Expect(handler.Save(stateFixture))
			Expect(handler.Load()).To(Equal(stateFixture))
		})
		It("should return no error if file already exists", func() {
			bytes := []byte("I am going to be overwritten")
			Expect(fs.WriteFile(testStatePath, bytes, 0700)).ToNot(HaveOccurred())
			Expect(handler.Save(stateFixture))
			Expect(handler.Load()).To(Equal(stateFixture))
		})
	})
	When("missing init handler", func() {
		It("should return error", func() {
			Expect(handler.Save(stateFixture)).To(HaveOccurred())
		})
	})
})
