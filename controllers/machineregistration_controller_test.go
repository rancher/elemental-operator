package controllers

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/test"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("reconcile machine registration", func() {
	var r *MachineRegistrationReconciler
	var mRegistration *elementalv1.MachineRegistration
	var setting *managementv3.Setting

	BeforeEach(func() {
		r = &MachineRegistrationReconciler{
			Client: cl,
		}

		mRegistration = &elementalv1.MachineRegistration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
		}

		setting = &managementv3.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name: "server-url",
			},
			Value: "https://example.com",
		}

		Expect(cl.Create(ctx, mRegistration)).To(Succeed())

		Expect(cl.Create(ctx, setting)).To(Succeed())
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, mRegistration, setting)).To(Succeed())
	})

	It("should reconcile machine registration object", func() {
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mRegistration.Namespace,
				Name:      mRegistration.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      mRegistration.Name,
			Namespace: mRegistration.Namespace,
		}, mRegistration)).To(Succeed())

		Expect(mRegistration.Finalizers).To(HaveLen(1))
		Expect(mRegistration.Finalizers[0]).To(Equal(elementalv1.MachineRegistrationFinalizer))
		Expect(mRegistration.Status.RegistrationToken).ToNot(BeEmpty())
		Expect(mRegistration.Status.RegistrationURL).To(ContainSubstring("https://example.com/elemental/registration/"))
		Expect(mRegistration.Status.ServiceAccountRef.Kind).To(Equal("ServiceAccount"))
		Expect(mRegistration.Status.ServiceAccountRef.Name).To(Equal(mRegistration.Name))
		Expect(mRegistration.Status.ServiceAccountRef.Namespace).To(Equal(mRegistration.Namespace))
		Expect(mRegistration.Status.Conditions).To(HaveLen(1))
		Expect(mRegistration.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(mRegistration.Status.Conditions[0].Reason).To(Equal(elementalv1.SuccefullyCreatedReason))
		Expect(mRegistration.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))

		objKey := types.NamespacedName{Namespace: mRegistration.Namespace, Name: mRegistration.Name}
		Expect(r.Get(ctx, objKey, &rbacv1.Role{})).To(Succeed())
		Expect(r.Get(ctx, objKey, &corev1.ServiceAccount{})).To(Succeed())
		Expect(r.Get(ctx, objKey, &rbacv1.RoleBinding{})).To(Succeed())
	})

	It("should reconcile machine registration object with a deletion timestamp", func() {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mRegistration.Namespace,
				Name:      mRegistration.Name,
			},
		}
		_, err := r.Reconcile(ctx, req)
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Delete(ctx, mRegistration)).To(Succeed())

		_, err = r.Reconcile(ctx, req)
		Expect(err).ToNot(HaveOccurred())

		Expect(mRegistration.Finalizers).To(HaveLen(0))
		objKey := types.NamespacedName{Namespace: mRegistration.Namespace, Name: mRegistration.Name}
		Expect(r.Get(ctx, objKey, &rbacv1.Role{})).ToNot(Succeed())
		Expect(r.Get(ctx, objKey, &corev1.ServiceAccount{})).ToNot(Succeed())
		Expect(r.Get(ctx, objKey, &rbacv1.RoleBinding{})).ToNot(Succeed())
	})
})

var _ = Describe("setRegistrationTokenAndUrl", func() {
	var r *MachineRegistrationReconciler
	var mRegistration *elementalv1.MachineRegistration

	BeforeEach(func() {
		r = &MachineRegistrationReconciler{
			Client: cl,
		}

		mRegistration = &elementalv1.MachineRegistration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
		}
	})

	It("should successfully set registration token and url", func() {
		setting := &managementv3.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name: "server-url",
			},
			Value: "https://example.com",
		}
		Expect(cl.Create(ctx, setting)).To(Succeed())
		Expect(r.setRegistrationTokenAndUrl(ctx, mRegistration)).To(Succeed())
		Expect(mRegistration.Status.RegistrationToken).ToNot(BeEmpty())
		Expect(mRegistration.Status.RegistrationURL).To(ContainSubstring("https://example.com/elemental/registration/"))
		Expect(test.CleanupAndWait(ctx, cl, setting)).To(Succeed())
	})

	It("should return error when setting doesn't exist", func() {
		err := r.setRegistrationTokenAndUrl(ctx, mRegistration)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get server url setting"))
	})

	It("should return error when setting doesn't have a value", func() {
		setting := &managementv3.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name: "server-url",
			},
		}
		Expect(cl.Create(ctx, setting)).To(Succeed())
		err := r.setRegistrationTokenAndUrl(ctx, mRegistration)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("server-url is not set"))
		Expect(test.CleanupAndWait(ctx, cl, setting)).To(Succeed())
	})
})

var _ = Describe("createRBACObjects", func() {
	var r *MachineRegistrationReconciler
	var mRegistration *elementalv1.MachineRegistration
	var role *rbacv1.Role
	var sa *corev1.ServiceAccount
	var roleBinding *rbacv1.RoleBinding

	BeforeEach(func() {
		r = &MachineRegistrationReconciler{
			Client: cl,
		}

		mRegistration = &elementalv1.MachineRegistration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
				UID:       "test",
			},
		}

		objMeta := metav1.ObjectMeta{Namespace: mRegistration.Namespace, Name: mRegistration.Name}
		role = &rbacv1.Role{ObjectMeta: objMeta}
		sa = &corev1.ServiceAccount{ObjectMeta: objMeta}
		roleBinding = &rbacv1.RoleBinding{
			ObjectMeta: objMeta,
			RoleRef: rbacv1.RoleRef{
				Kind:     "Role",
				Name:     mRegistration.Name,
				APIGroup: "rbac.authorization.k8s.io",
			},
		}
	})

	AfterEach(func() {
		test.CleanupAndWait(ctx, cl, role, sa, roleBinding)
	})

	It("should successfully create RBAC objects", func() {
		Expect(r.createRBACObjects(ctx, mRegistration)).To(Succeed())
		objKey := types.NamespacedName{Namespace: mRegistration.Namespace, Name: mRegistration.Name}
		role := &rbacv1.Role{}
		Expect(r.Get(ctx, objKey, role)).To(Succeed())

		Expect(role.OwnerReferences).To(HaveLen(1))
		Expect(role.OwnerReferences[0].APIVersion).To(Equal(elementalv1.GroupVersion.String()))
		Expect(role.OwnerReferences[0].Kind).To(Equal("MachineRegistration"))
		Expect(role.OwnerReferences[0].Name).To(Equal(mRegistration.Name))
		Expect(role.OwnerReferences[0].UID).To(Equal(mRegistration.UID))
		Expect(role.OwnerReferences[0].Controller).To(Equal(pointer.Bool(true)))

		Expect(role.Rules).To(HaveLen(2))
		Expect(role.Rules[0].APIGroups).To(Equal([]string{""}))
		Expect(role.Rules[0].Verbs).To(Equal([]string{"get", "watch", "list", "update", "patch"}))
		Expect(role.Rules[0].Resources).To(Equal([]string{"secrets"}))
		Expect(role.Rules[1].APIGroups).To(Equal([]string{"management.cattle.io"}))
		Expect(role.Rules[1].Verbs).To(Equal([]string{"get", "watch", "list"}))
		Expect(role.Rules[1].Resources).To(Equal([]string{"settings"}))

		sa := &corev1.ServiceAccount{}
		Expect(r.Get(ctx, objKey, sa)).To(Succeed())
		Expect(sa.OwnerReferences).To(HaveLen(1))
		Expect(sa.OwnerReferences[0].APIVersion).To(Equal(elementalv1.GroupVersion.String()))
		Expect(sa.OwnerReferences[0].Kind).To(Equal("MachineRegistration"))
		Expect(sa.OwnerReferences[0].Name).To(Equal(mRegistration.Name))
		Expect(sa.OwnerReferences[0].UID).To(Equal(mRegistration.UID))
		Expect(sa.OwnerReferences[0].Controller).To(Equal(pointer.Bool(true)))

		roleBinding := &rbacv1.RoleBinding{}
		Expect(r.Get(ctx, objKey, roleBinding)).To(Succeed())
		Expect(roleBinding.OwnerReferences).To(HaveLen(1))
		Expect(roleBinding.OwnerReferences[0].APIVersion).To(Equal(elementalv1.GroupVersion.String()))
		Expect(roleBinding.OwnerReferences[0].Kind).To(Equal("MachineRegistration"))
		Expect(roleBinding.OwnerReferences[0].Name).To(Equal(mRegistration.Name))
		Expect(roleBinding.OwnerReferences[0].UID).To(Equal(mRegistration.UID))
		Expect(roleBinding.OwnerReferences[0].Controller).To(Equal(pointer.Bool(true)))

		Expect(roleBinding.Subjects).To(HaveLen(1))
		Expect(roleBinding.Subjects[0].Kind).To(Equal("ServiceAccount"))
		Expect(roleBinding.Subjects[0].Name).To(Equal(mRegistration.Name))
		Expect(roleBinding.Subjects[0].Namespace).To(Equal(mRegistration.Namespace))

		Expect(mRegistration.Status.ServiceAccountRef.Kind).To(Equal("ServiceAccount"))
		Expect(mRegistration.Status.ServiceAccountRef.Name).To(Equal(mRegistration.Name))
		Expect(mRegistration.Status.ServiceAccountRef.Namespace).To(Equal(mRegistration.Namespace))

	})

	It("shouldn't error when RBAC already exists", func() {
		Expect(r.Create(ctx, role)).To(Succeed())
		Expect(r.Create(ctx, sa)).To(Succeed())
		Expect(r.Create(ctx, roleBinding)).To(Succeed())
		Expect(r.createRBACObjects(ctx, mRegistration)).To(Succeed())
		Expect(mRegistration.Status.ServiceAccountRef.Kind).To(Equal("ServiceAccount"))
		Expect(mRegistration.Status.ServiceAccountRef.Name).To(Equal(mRegistration.Name))
		Expect(mRegistration.Status.ServiceAccountRef.Namespace).To(Equal(mRegistration.Namespace))
	})

	It("should error when RBAC fails to be created", func() {
		r.Client = machineRegistrationFailingClient{}
		err := r.createRBACObjects(ctx, mRegistration)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create"))
	})
})

var _ = Describe("reconcileDelete", func() {
	var r *MachineRegistrationReconciler
	var mRegistration *elementalv1.MachineRegistration
	var role *rbacv1.Role
	var sa *corev1.ServiceAccount
	var roleBinding *rbacv1.RoleBinding

	BeforeEach(func() {
		r = &MachineRegistrationReconciler{
			Client: cl,
		}

		mRegistration = &elementalv1.MachineRegistration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
				UID:       "test",
			},
		}

		objMeta := metav1.ObjectMeta{Namespace: mRegistration.Namespace, Name: mRegistration.Name}
		role = &rbacv1.Role{ObjectMeta: objMeta}
		sa = &corev1.ServiceAccount{ObjectMeta: objMeta}
		roleBinding = &rbacv1.RoleBinding{
			ObjectMeta: objMeta,
			RoleRef: rbacv1.RoleRef{
				Kind:     "Role",
				Name:     mRegistration.Name,
				APIGroup: "rbac.authorization.k8s.io",
			},
		}
	})

	AfterEach(func() {
		test.CleanupAndWait(ctx, cl, role, sa, roleBinding)
	})

	It("should successfully delete RBAC objects", func() {
		controllerutil.AddFinalizer(mRegistration, elementalv1.MachineRegistrationFinalizer)
		Expect(r.createRBACObjects(ctx, mRegistration)).To(Succeed())
		_, err := r.reconcileDelete(ctx, mRegistration)
		Expect(err).ToNot(HaveOccurred())
		objKey := types.NamespacedName{Namespace: mRegistration.Namespace, Name: mRegistration.Name}
		Expect(r.Get(ctx, objKey, &rbacv1.Role{})).ToNot(Succeed())
		Expect(r.Get(ctx, objKey, &corev1.ServiceAccount{})).ToNot(Succeed())
		Expect(r.Get(ctx, objKey, &rbacv1.RoleBinding{})).ToNot(Succeed())
		Expect(mRegistration.Finalizers).To(HaveLen(0))
	})

	It("shouldn't error when RBAC already deleted", func() {
		Expect(r.createRBACObjects(ctx, mRegistration)).To(Succeed())
		_, err := r.reconcileDelete(ctx, mRegistration)
		Expect(err).ToNot(HaveOccurred())
		_, err = r.reconcileDelete(ctx, mRegistration)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should error when RBAC fails to be deleted", func() {
		r.Client = machineRegistrationFailingClient{}
		_, err := r.reconcileDelete(ctx, mRegistration)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to delete"))
	})
})

type machineRegistrationFailingClient struct {
	client.Client
}

func (cl machineRegistrationFailingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return errors.New("failed to create")
}

func (cl machineRegistrationFailingClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return errors.New("failed to delete")
}
