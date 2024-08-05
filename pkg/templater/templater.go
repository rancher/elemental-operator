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

package templater

import (
	"errors"
	"strings"

	values "github.com/rancher/wrangler/v2/pkg/data"
)

var errValueNotFound = errors.New("value not found")

func IsValueNotFoundError(err error) bool {
	return errors.Is(err, errValueNotFound)
}

type Templater interface {
	Fill(data map[string]interface{})
	Decode(str string) (string, error)
}

func NewTemplater() Templater {
	return &templater{
		data: map[string]interface{}{},
	}
}

var _ Templater = (*templater)(nil)

type templater struct {
	data map[string]interface{}
}

func (t *templater) Fill(data map[string]interface{}) {
	for k, v := range data {
		t.data[k] = v
	}
}

func (t *templater) Decode(str string) (string, error) {
	return replaceStringData(t.data, str)
}

func replaceStringData(data map[string]interface{}, name string) (string, error) {
	str := name
	result := &strings.Builder{}
	for {
		i := strings.Index(str, "${")
		if i == -1 {
			result.WriteString(str)
			break
		}
		j := strings.Index(str[i:], "}")
		if j == -1 {
			result.WriteString(str)
			break
		}

		result.WriteString(str[:i])
		obj := values.GetValueN(data, strings.Split(str[i+2:j+i], "/")...)
		if str, ok := obj.(string); ok {
			result.WriteString(str)
		} else {
			return "", errValueNotFound
		}
		str = str[j+i+1:]
	}

	return result.String(), nil
}
