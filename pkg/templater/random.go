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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// Random template format examples:
// Random/UUID		--> e511d5ca-a765-42f2-82b7-264f37ffb329
// Random/Hex/3		--> e512
// Random/Int/5		--> 75423
const (
	tmplRandomKey = "Random"
	tmplUUIDKey   = "UUID"
	tmplHexKey    = "Hex"
	tmplIntKey    = "Int"
)

func isRandomTemplate(tmplVal []string) bool {
	if tmplVal[0] != tmplRandomKey {
		return false
	}

	switch tmplVal[1] {
	case tmplUUIDKey:
	case tmplHexKey:
	case tmplIntKey:
	default:
		return false
	}

	return true
}

func randomTemplateToString(tmplVal []string) (string, error) {
	// expected tamplates:
	// 	Random/UUID
	// 	Random/Hex/[1-32]
	//	Random/Int/[MAXINT]
	// examples:
	//	[Random][UUID]		--> e511d5ca-a765-42f2-82b7-264f37ffb329
	//  [Random][Hex][3]	--> e512
	//  [Random][Hex][11]	--> e512d5caa765
	//	[Random][Int][100]-->	24

	if !isRandomTemplate(tmplVal) {
		return "", errValueNotFound
	}

	tmplLen := len(tmplVal)
	if tmplLen > 3 || tmplLen < 2 {
		return "", fmt.Errorf("invalid template: %s", strings.Join(tmplVal, "/"))
	}

	switch tmplVal[1] {
	case tmplUUIDKey:
		if len(tmplVal) != 2 {
			return "", fmt.Errorf("invalid template: %s", strings.Join(tmplVal, "/"))
		}
		return uuid.NewString(), nil
	case tmplHexKey:
		if len(tmplVal) != 3 {
			return "", fmt.Errorf("invalid template: %s", strings.Join(tmplVal, "/"))
		}
		rndLen, err := strconv.Atoi(tmplVal[2])
		if err != nil || rndLen < 1 {
			return "", fmt.Errorf("unsupported %s/%s template: %s",
				tmplRandomKey, tmplUUIDKey, strings.Join(tmplVal, "/"))
		}
		if rndLen > 32 {
			return "", fmt.Errorf("unsupported %s/%s lenght: %s",
				tmplRandomKey, tmplUUIDKey, strings.Join(tmplVal, "/"))
		}

		rndHex := make([]byte, 32)
		uuid := uuid.New()
		hex.Encode(rndHex, uuid[:])
		return string(rndHex[:rndLen]), nil
	case tmplIntKey:
		if len(tmplVal) != 3 {
			return "", fmt.Errorf("invalid template: %s", strings.Join(tmplVal, "/"))
		}
		intMax, err := strconv.Atoi(tmplVal[2])
		if err != nil || intMax < 1 {
			return "", fmt.Errorf("unsupported %s/%s template: %s",
				tmplRandomKey, tmplIntKey, strings.Join(tmplVal, "/"))
		}
		intBigVal, err := rand.Int(rand.Reader, big.NewInt(int64(intMax)))
		if err != nil {
			return "", fmt.Errorf("converting %s: %w", strings.Join(tmplVal, "/"), err)
		}
		strVal := fmt.Sprintf("%d", intBigVal)
		return strVal, nil
	}

	return "", fmt.Errorf("invalid template: %s", strings.Join(tmplVal, "/"))
}
