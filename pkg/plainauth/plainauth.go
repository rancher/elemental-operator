/*
Copyright © 2022 - 2025 SUSE LLC

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

package plainauth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/log"
)

type AuthServer struct {
	context.Context
	client.Client
}

func New(ctx context.Context, cl client.Client) *AuthServer {
	a := &AuthServer{
		Context: ctx,
		Client:  cl,
	}

	return a
}

func (a *AuthServer) Authenticate(_ *websocket.Conn, req *http.Request, regNamespace string) (*elementalv1.MachineInventory, bool, error) {
	header := req.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer PLAIN") {
		log.Debugf("websocket connection missing PLAIN Authorization header from %s", req.RemoteAddr)
		return nil, true, nil
	}

	log.Info("Authentication: PLAIN")
	idToken := strings.TrimPrefix(header, "Bearer PLAIN")
	if idToken == "" {
		return nil, false, fmt.Errorf("cannot find token ID")
	}
	hashedToken := fmt.Sprintf("%x", sha256.Sum256([]byte(idToken)))

	mInvetoryList := &elementalv1.MachineInventoryList{}
	if err := a.List(a, mInvetoryList); err != nil {
		return nil, false, fmt.Errorf("failed to get MachineInventories list: %w", err)
	}

	var mInventory *elementalv1.MachineInventory
	for _, m := range mInvetoryList.Items {
		if m.Spec.MachineHash == hashedToken {
			// If we get two MachineInventory with the same MachineHash something went wrong
			if mInventory != nil {
				return nil, false, fmt.Errorf("failed to find inventory machine: Machine hash %s is present in both %s/%s and %s/%s",
					hashedToken, mInventory.Namespace, mInventory.Name, m.Namespace, m.Name)
			}
			mInventory = (&m).DeepCopy()
		}
	}

	if mInventory == nil {
		mInventory = &elementalv1.MachineInventory{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: regNamespace,
			},
			Spec: elementalv1.MachineInventorySpec{
				MachineHash: hashedToken,
			},
		}
	}

	return mInventory, false, nil
}
