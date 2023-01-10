package image

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/rancher/wharfie/pkg/credentialprovider/plugin"
	"github.com/rancher/wharfie/pkg/registries"
	"github.com/rancher/wharfie/pkg/tarfile"
	"github.com/sirupsen/logrus"
)

const (
	baseRancherDir                       string = "/var/lib/rancher/"
	defaultImagesDir                            = baseRancherDir + "agent/images"
	defaultImageCredentialProviderConfig        = baseRancherDir + "credentialprovider/config.yaml"
	defaultImageCredentialProviderBinDir        = baseRancherDir + "credentialprovider/bin"
	defaultAgentRegistriesFile           string = "/etc/rancher/agent/registries.yaml"
	rke2RegistriesFile                   string = "/etc/rancher/rke2/registries.yaml"
	k3sRegistriesFile                    string = "/etc/rancher/k3s/registries.yaml"
)

type Utility struct {
	imagesDir                     string
	imageCredentialProviderConfig string
	imageCredentialProviderBinDir string
	agentRegistriesFile           string
}

func NewUtility(imagesDir, imageCredentialProviderConfig, imageCredentialProviderBinDir, agentRegistriesFile string) *Utility {
	var u Utility

	if imagesDir != "" {
		u.imagesDir = imagesDir
	} else {
		u.imagesDir = defaultImagesDir
	}

	if imageCredentialProviderConfig != "" {
		u.imageCredentialProviderConfig = imageCredentialProviderConfig
	} else {
		u.imageCredentialProviderConfig = defaultImageCredentialProviderConfig
	}

	if imageCredentialProviderBinDir != "" {
		u.imageCredentialProviderBinDir = imageCredentialProviderBinDir
	} else {
		u.imageCredentialProviderBinDir = defaultImageCredentialProviderBinDir
	}

	if agentRegistriesFile != "" {
		u.agentRegistriesFile = agentRegistriesFile
	} else {
		u.agentRegistriesFile = defaultAgentRegistriesFile
	}

	logrus.Debugf("Instantiated new image utility with imagesDir: %s, imageCredentialProviderConfig: %s, imageCredentialProviderBinDir: %s, agentRegistriesFile: %s", u.imagesDir, u.imageCredentialProviderConfig, u.imageCredentialProviderBinDir, u.agentRegistriesFile)

	return &u
}

func (u *Utility) Stage(destDir string, imgString string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	var img v1.Image
	image, err := name.ParseReference(imgString)
	if err != nil {
		return err
	}

	imagesDir, err := filepath.Abs(u.imagesDir)
	if err != nil {
		return err
	}

	i, err := tarfile.FindImage(imagesDir, image)
	if err != nil && !errors.Is(err, tarfile.ErrNotFound) {
		return err
	}
	img = i

	if img == nil {
		registry, err := registries.GetPrivateRegistries(u.findRegistriesYaml())
		if err != nil {
			return err
		}

		if _, err := os.Stat(u.imageCredentialProviderConfig); os.IsExist(err) {
			logrus.Debugf("Image Credential Provider Configuration file %s existed, using plugins from directory %s", u.imageCredentialProviderConfig, u.imageCredentialProviderBinDir)
			plugins, err := plugin.RegisterCredentialProviderPlugins(u.imageCredentialProviderConfig, u.imageCredentialProviderBinDir)
			if err != nil {
				return err
			}
			registry.DefaultKeychain = plugins
		} else {
			// The kubelet image credential provider plugin also falls back to checking legacy Docker credentials, so only
			// explicitly set up the go-containerregistry DefaultKeychain if plugins are not configured.
			// DefaultKeychain tries to read config from the home dir, and will error if HOME isn't set, so also gate on that.
			if os.Getenv("HOME") != "" {
				registry.DefaultKeychain = authn.DefaultKeychain
			}
		}

		logrus.Infof("Pulling image %s", image.Name())
		img, err = registry.Image(image,
			remote.WithPlatform(v1.Platform{
				Architecture: runtime.GOARCH,
				OS:           runtime.GOOS,
			}),
		)
		if err != nil {
			return fmt.Errorf("%v: failed to get image %s", err, image.Name())
		}
	}

	return extractFiles(img, destDir)
}

func (u *Utility) findRegistriesYaml() string {
	if _, err := os.Stat(u.agentRegistriesFile); err == nil {
		return u.agentRegistriesFile
	}
	if _, err := os.Stat(rke2RegistriesFile); err == nil {
		return rke2RegistriesFile
	}
	if _, err := os.Stat(k3sRegistriesFile); err == nil {
		return k3sRegistriesFile
	}
	return ""
}
