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

package install

import (
	"os/exec"
	"strings"

	"github.com/pkg/errors"
	"github.com/rancher/elemental-operator/pkg/config"
	"github.com/rancher/elemental-operator/pkg/questions"
	"github.com/rancher/elemental-operator/pkg/util"
)

func Ask(cfg *config.Config) error {
	if err := AskInstallDevice(cfg); err != nil {
		return err
	}

	if err := AskConfigURL(cfg); err != nil {
		return err
	}

	if cfg.Elemental.Install.ConfigURL == "" {
		if err := AskGithub(cfg); err != nil {
			return err
		}

		if err := AskPassword(cfg); err != nil {
			return err
		}

		if err := AskServerAgent(cfg); err != nil {
			return err
		}
	}

	return nil
}

func AskInstallDevice(cfg *config.Config) error {
	if cfg.Elemental.Install.Device != "" {
		return nil
	}

	if cfg.Elemental.Install.Automatic {
		return errors.Errorf("Target disk was not provided, please add device option to config file (e.g. device: /dev/sdb)")
	}

	output, err := exec.Command("/bin/sh", "-c", "lsblk -r -o NAME,TYPE | awk '/disk|loop/ {if ($1 != \"fd0\") {print $1}}'").CombinedOutput()
	if err != nil {
		return err
	}
	fields := strings.Fields(string(output))
	i, err := questions.PromptFormattedOptions("Installation target. WARNING: Device will be formatted !!!)", -1, fields...)
	if err != nil {
		return err
	}

	cfg.Elemental.Install.Device = "/dev/" + fields[i]
	return nil
}

func AskToken(cfg *config.Config, server bool) error {
	var (
		token string
		err   error
	)

	if cfg.Elemental.Install.Token != "" {
		return nil
	}

	msg := "Token or cluster secret"
	if server {
		msg += " (optional)"
	}
	if server {
		token, err = questions.PromptOptional(msg+": ", "")
	} else {
		token, err = questions.Prompt(msg+": ", "")
	}
	cfg.Elemental.Install.Token = token

	return err
}

func isServer() (bool, bool, error) {
	opts := []string{"server", "agent", "none"}
	i, err := questions.PromptFormattedOptions("Run as server or agent (Choose none if building an image)?", 0, opts...)
	if err != nil {
		return false, false, err
	}

	return i == 0, i == 1, nil
}

func AskServerAgent(cfg *config.Config) error {
	if cfg.Elemental.Install.ServerURL != "" || cfg.Elemental.Install.Automatic {
		return nil
	}

	server, agent, err := isServer()
	if err != nil {
		return err
	}

	if !server && !agent {
		return nil
	}

	if server {
		cfg.Elemental.Install.Role = "server"
		return AskToken(cfg, true)
	}

	cfg.Elemental.Install.Role = "agent"
	url, err := questions.Prompt("URL of server: ", "")
	if err != nil {
		return err
	}
	cfg.Elemental.Install.ServerURL = url

	return AskToken(cfg, false)
}

func AskPassword(cfg *config.Config) error {
	if cfg.Elemental.Install.Automatic || cfg.Elemental.Install.Password != "" {
		return nil
	}

	var (
		ok   = false
		err  error
		pass string
	)

	for !ok {
		pass, ok, err = util.PromptPassword()
		if err != nil {
			return err
		}
	}

	if pass != "" {
		pass, err = util.GetEncryptedPasswd(pass)
		if err != nil {
			return err
		}
	}

	cfg.Elemental.Install.Password = pass
	return nil
}

func AskGithub(cfg *config.Config) error {
	if len(cfg.SSHAuthorizedKeys) > 0 || cfg.Elemental.Install.Password != "" || cfg.Elemental.Install.Automatic {
		return nil
	}

	ok, err := questions.PromptBool("Authorize GitHub users to root SSH?", false)
	if !ok || err != nil {
		return err
	}

	str, err := questions.Prompt("Comma separated list of GitHub users to authorize: ", "")
	if err != nil {
		return err
	}

	for _, s := range strings.Split(str, ",") {
		cfg.SSHAuthorizedKeys = append(cfg.SSHAuthorizedKeys, "github:"+strings.TrimSpace(s))
	}

	return nil
}

func AskConfigURL(cfg *config.Config) error {
	if cfg.Elemental.Install.ConfigURL != "" || cfg.Elemental.Install.Automatic {
		return nil
	}

	ok, err := questions.PromptBool("Configure system using an cloud-config file?", false)
	if err != nil {
		return err
	}

	if !ok {
		return nil
	}

	str, err := questions.Prompt("cloud-config file location (file path or http URL): ", "")
	if err != nil {
		return err
	}

	cfg.Elemental.Install.ConfigURL = str
	return nil
}
