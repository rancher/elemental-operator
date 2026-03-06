/*
Copyright © 2022 - 2026 SUSE LLC

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

package tpm

import (
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/go-attestation/attest"

	gotpm "github.com/rancher-sandbox/go-tpm"

	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/log"
)

func (a *AuthServer) verifyChain(ek *attest.EK, namespace string) error {
	secret := &corev1.Secret{}
	err := a.Get(a, types.NamespacedName{
		Name:      tpmCACert,
		Namespace: namespace,
	}, secret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get secret: %w", err)
	}

	roots := x509.NewCertPool()
	_ = roots.AppendCertsFromPEM(secret.Data[corev1.TLSCertKey])
	opts := x509.VerifyOptions{
		Roots: roots,
	}
	_, err = ek.Certificate.Verify(opts)
	return err
}

func (a *AuthServer) validHash(ek *attest.EK, registerNamespace string) (*elementalv1.MachineInventory, error) {
	hashEncoded, err := gotpm.DecodePubHash(ek)
	if err != nil {
		return nil, fmt.Errorf("tpm: could not get public key hash: %v", err)
	}

	if err := a.verifyChain(ek, registerNamespace); err != nil {
		return nil, fmt.Errorf("verifying chain: %w", err)
	}

	mInventoryList := &elementalv1.MachineInventoryList{}
	if err := a.List(a, mInventoryList); err != nil {
		return nil, fmt.Errorf("failed to get MachineInventories list: %w", err)
	}

	// TODO: build and keep a MachineInventory hash-map indexed on TPMHash
	var mInventory *elementalv1.MachineInventory

	for _, m := range mInventoryList.Items {
		if m.Spec.TPMHash == hashEncoded {
			// If we get two MachineInventory with same TPM something got bad...
			if mInventory != nil {
				return nil, fmt.Errorf("failed to find inventory machine: TPM hash %s is present in both %s/%s and %s/%s",
					hashEncoded, mInventory.Namespace, mInventory.Name, m.Namespace, m.Name)
			}
			mInventory = (&m).DeepCopy()
		}
	}

	if mInventory == nil {
		mInventory = &elementalv1.MachineInventory{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: registerNamespace,
			},
			Spec: elementalv1.MachineInventorySpec{
				TPMHash: hashEncoded,
			},
		}
	}

	return mInventory, nil
}

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

	return io.ReadAll(reader)
}

func (a *AuthServer) Authenticate(conn *websocket.Conn, req *http.Request, registerNamespace string) (*elementalv1.MachineInventory, bool, error) {
	header := req.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer TPM") {
		log.Debug("websocket connection: no TPM Authorization header")
		return nil, true, nil
	}

	log.Info("Authentication: TPM")

	ek, attestationData, err := gotpm.GetAttestationData(header)
	if err != nil {
		return nil, false, err
	}

	machine, err := a.validHash(ek, registerNamespace)
	if err != nil {
		return nil, false, err
	}

	secret, challenge, err := gotpm.GenerateChallenge(ek, attestationData)
	if err != nil {
		return nil, false, err
	}

	challResp, err := writeRead(conn, challenge)
	if err != nil {
		return nil, false, err
	}

	if err := gotpm.ValidateChallenge(secret, challResp); err != nil {
		return nil, false, err
	}

	return machine, false, nil
}
