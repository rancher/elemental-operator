package tpm

import (
	"testing"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFindMachineInventoryForHash(t *testing.T) {
	testCases := []struct {
		name                 string
		encodedHash          string
		existingMInventories []*elementalv1.MachineInventory
		expectedError        bool
	}{
		{
			name:        "succesfully find a machine inventory for hash",
			encodedHash: "test",
			existingMInventories: []*elementalv1.MachineInventory{
				{
					ObjectMeta: v1.ObjectMeta{
						Name:      "name1",
						Namespace: "namespace",
					},
					Spec: elementalv1.MachineInventorySpec{
						TPMHash: "test1",
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Name:      "name2",
						Namespace: "namespace",
					},
					Spec: elementalv1.MachineInventorySpec{
						TPMHash: "test",
					},
				},
			},
		},
		{
			name:        "fail when two machine inventories with similar hash exist",
			encodedHash: "test",
			existingMInventories: []*elementalv1.MachineInventory{
				{
					ObjectMeta: v1.ObjectMeta{
						Name:      "name1",
						Namespace: "namespace",
					},
					Spec: elementalv1.MachineInventorySpec{
						TPMHash: "test1",
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Name:      "name2",
						Namespace: "namespace",
					},
					Spec: elementalv1.MachineInventorySpec{
						TPMHash: "test",
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Name:      "name3",
						Namespace: "namespace",
					},
					Spec: elementalv1.MachineInventorySpec{
						TPMHash: "test",
					},
				},
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			elementalv1.AddToScheme(scheme)

			objs := []runtime.Object{}
			for _, mInventory := range tc.existingMInventories {
				objs = append(objs, mInventory)
			}
			authServer := AuthServer{
				Client: fake.NewFakeClientWithScheme(scheme, objs...),
			}

			_, err := authServer.findMachineInventoryForHash(tc.encodedHash, "namespace")
			if tc.expectedError {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("did not expect error")
				}
			}
		})
	}
}
