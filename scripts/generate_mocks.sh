#!/bin/sh

go install go.uber.org/mock/mockgen@v0.4.0

mockgen -copyright_file=scripts/boilerplate.go.txt -destination=pkg/register/mocks/client.go -package=mocks github.com/rancher/elemental-operator/pkg/register Client
mockgen -copyright_file=scripts/boilerplate.go.txt -destination=pkg/register/mocks/state.go -package=mocks github.com/rancher/elemental-operator/pkg/register StateHandler
mockgen -copyright_file=scripts/boilerplate.go.txt -destination=pkg/install/mocks/install.go -package=mocks github.com/rancher/elemental-operator/pkg/install Installer
mockgen -copyright_file=scripts/boilerplate.go.txt -destination=pkg/elementalcli/mocks/elementalcli.go -package=mocks github.com/rancher/elemental-operator/pkg/elementalcli Runner
mockgen -copyright_file=scripts/boilerplate.go.txt -destination=pkg/network/mocks/network.go -package=mocks github.com/rancher/elemental-operator/pkg/network Configurator
