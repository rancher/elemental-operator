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

package operator

type options struct {
	namespace     string
	services      []service
	operatorImage string
	requeuer      chan interface{}
}

// Setting are settings for the operator
type Setting func(o *options) error

func (o *options) apply(settings ...Setting) error {
	for _, opt := range settings {
		err := opt(o)
		if err != nil {
			return err
		}
	}
	return nil
}

// WithNamespace sets the operator working namespace
func WithNamespace(s string) Setting {
	return func(o *options) error {
		o.namespace = s
		return nil
	}
}

// WithRequeuer sets the operator requeuer channel for ManagedOSVersions syncs
func WithRequeuer(c chan interface{}) Setting {
	return func(o *options) error {
		o.requeuer = c
		return nil
	}
}

// WithOperatorImage sets the operator image
func WithOperatorImage(s string) Setting {
	return func(o *options) error {
		o.operatorImage = s
		return nil
	}
}

// WithServices sets the operator services
func WithServices(s ...service) Setting {
	return func(o *options) error {
		o.services = append(o.services, s...)
		return nil
	}
}
