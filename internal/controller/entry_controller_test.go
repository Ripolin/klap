/*
Copyright 2025.

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

package controller

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"

	klapv1alpha1 "github.com/ripolin/klap/api/v1alpha1"
	"github.com/ripolin/klap/test/mock_ldap"
	gomock "go.uber.org/mock/gomock"

	"github.com/go-ldap/ldap/v3"
)

var _ = Describe("Entry Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-entry"
		const serverName = "my-ldap-server"
		const passwdName = "my-passwd"
		const uuid = "fab03ddc-1989-471d-84f3-4c19d32fda35"

		var ns = "default"
		var dn = "cn=foobar"

		var mockClient *mock_ldap.MockClient

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: ns,
		}

		serverTypeNamespacedName := types.NamespacedName{
			Name:      serverName,
			Namespace: ns,
		}

		passwdTypeNamespacedName := types.NamespacedName{
			Name:      passwdName,
			Namespace: ns,
		}

		entry := &klapv1alpha1.Entry{}
		server := &klapv1alpha1.Server{}
		passwd := &corev1.Secret{}

		BeforeEach(func() {
			mockCtrl := gomock.NewController(GinkgoT())
			mockClient = mock_ldap.NewMockClient(mockCtrl)

			By("creating the custom resource for the Kind Secret")
			err := k8sClient.Get(ctx, passwdTypeNamespacedName, passwd)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      passwdName,
						Namespace: ns,
					},
					StringData: map[string]string{
						"password": "foobar",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("creating the custom resource for the Kind Server")
			err = k8sClient.Get(ctx, serverTypeNamespacedName, server)
			if err != nil && errors.IsNotFound(err) {
				baseDN := "foo"
				bindDN := "bar"
				ldapUrl := "http://my-ldap-server"
				passwordName := passwdName
				key := "password"
				resource := &klapv1alpha1.Server{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serverName,
						Namespace: ns,
					},
					Spec: klapv1alpha1.ServerSpec{
						BaseDN: &baseDN,
						BindDN: &bindDN,
						Url:    &ldapUrl,
						PasswordSecretRef: klapv1alpha1.SecretRef{
							Name: &passwordName,
							Key:  &key,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("creating the custom resource for the Kind Entry")
			err = k8sClient.Get(ctx, typeNamespacedName, entry)
			if err != nil && errors.IsNotFound(err) {
				sName := serverName
				resource := &klapv1alpha1.Entry{
					ObjectMeta: metav1.ObjectMeta{
						Name:       resourceName,
						Namespace:  ns,
						Finalizers: []string{Finalizer},
					},
					Spec: klapv1alpha1.EntrySpec{
						DN:    &dn,
						Prune: true,
						Force: false,
						Attributes: map[string][]string{
							"objectClass": {"groupOfNames"},
							"description": {"test"},
						},
						ServerRef: klapv1alpha1.ResourceRef{
							Name:      &sName,
							Namespace: &ns,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			// resource := &klapv1alpha1.Entry{}
			// err := k8sClient.Get(ctx, typeNamespacedName, resource)
			// Expect(err).NotTo(HaveOccurred())

			// By("Cleanup the specific resource instance Entry")
			// Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			serverResource := &klapv1alpha1.Server{}
			err := k8sClient.Get(ctx, serverTypeNamespacedName, serverResource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Server")
			Expect(k8sClient.Delete(ctx, serverResource)).To(Succeed())

			passwdResource := &corev1.Secret{}
			err = k8sClient.Get(ctx, passwdTypeNamespacedName, passwdResource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Secret")
			Expect(k8sClient.Delete(ctx, passwdResource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")

			mockClient.EXPECT().Bind(gomock.Any(), gomock.Any()).MaxTimes(3).Return(nil)
			mockClient.EXPECT().Add(gomock.Any()).Return(nil)
			mockClient.EXPECT().Search(gomock.Cond(func(search *ldap.SearchRequest) bool {
				return search.Filter == fmt.Sprintf("(%s=%s)", OpenLDAPDN, dn)
			})).Return(&ldap.SearchResult{
				Entries: []*ldap.Entry{
					{
						DN: dn,
						Attributes: []*ldap.EntryAttribute{
							{
								Name:   OpenLDAPGUID,
								Values: []string{uuid},
							},
						},
					},
				},
			}, nil)
			mockClient.EXPECT().Unbind().MaxTimes(3).Return(nil)

			controllerReconciler := &EntryReconciler{
				ldapClient: mockClient,
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				Recorder:   recorder,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
			entry := &klapv1alpha1.Entry{}
			err = k8sClient.Get(ctx, typeNamespacedName, entry)
			Expect(err).NotTo(HaveOccurred())
			Expect(entry.Status.GUID).NotTo(BeNil())

			By("Reconciling the existing resource")
			mockClient.EXPECT().Search(gomock.Cond(func(search *ldap.SearchRequest) bool {
				return search.Filter == fmt.Sprintf("(%s=%s)", OpenLDAPGUID, uuid)
			})).Return(&ldap.SearchResult{
				Entries: []*ldap.Entry{
					{
						DN: dn,
						Attributes: []*ldap.EntryAttribute{
							{
								Name:   "objectClass",
								Values: []string{"groupOfNames"},
							},
							{
								Name:   "description",
								Values: []string{"changed"},
							},
						},
					},
				},
			}, nil)
			mockClient.EXPECT().Modify(gomock.Any()).Return(nil)
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Deleting the existing resource")
			mockClient.EXPECT().Del(gomock.Cond(func(delete *ldap.DelRequest) bool {
				return delete.DN == dn
			})).Return(nil)
			err = k8sClient.Get(ctx, typeNamespacedName, entry)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, entry)).To(Succeed())
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When filtering entries by namespace", func() {
		const serverNamespace = "server-ns"

		ctx := context.Background()

		var reconciler *EntryReconciler

		// newServer builds a Server living in serverNamespace with the given selector.
		newServer := func(selector *klapv1alpha1.NamespaceSelector) *klapv1alpha1.Server {
			return &klapv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "filtered-server",
					Namespace: serverNamespace,
				},
				Spec: klapv1alpha1.ServerSpec{
					AllowedNamespaces: selector,
				},
			}
		}

		// newEntry builds an Entry living in the given namespace.
		newEntry := func(namespace string) *klapv1alpha1.Entry {
			return &klapv1alpha1.Entry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "filtered-entry",
					Namespace: namespace,
				},
			}
		}

		ptr := func(s string) *string { return &s }

		BeforeEach(func() {
			reconciler = &EntryReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}

			By("creating namespaces used by the label selector cases")
			for name, lbls := range map[string]map[string]string{
				"team-blue":   {"team": "blue"},
				"team-red":    {"team": "red"},
				"no-label-ns": nil,
			} {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   name,
						Labels: lbls,
					},
				}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, ns)
				if err != nil && errors.IsNotFound(err) {
					Expect(k8sClient.Create(ctx, ns)).To(Succeed())
				}
			}
		})

		It("allows an entry living in the same namespace as the server", func() {
			// Even with a selector that matches nothing, same-namespace is allowed.
			server := newServer(&klapv1alpha1.NamespaceSelector{NamePattern: ptr("nomatch")})
			entry := newEntry(serverNamespace)

			allowed, err := reconciler.isNamespaceAllowed(ctx, entry, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue())
		})

		It("denies a foreign namespace when no selector is configured", func() {
			server := newServer(nil)
			entry := newEntry("team-blue")

			allowed, err := reconciler.isNamespaceAllowed(ctx, entry, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeFalse())
		})

		It("allows the server's own namespace when no selector is configured", func() {
			server := newServer(nil)
			entry := newEntry(serverNamespace)

			allowed, err := reconciler.isNamespaceAllowed(ctx, entry, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue())
		})

		It("allows an entry whose namespace name matches the pattern", func() {
			server := newServer(&klapv1alpha1.NamespaceSelector{NamePattern: ptr("team-.*")})
			entry := newEntry("team-blue")

			allowed, err := reconciler.isNamespaceAllowed(ctx, entry, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue())
		})

		It("denies an entry whose namespace name does not match the pattern", func() {
			server := newServer(&klapv1alpha1.NamespaceSelector{NamePattern: ptr("team-.*")})
			entry := newEntry("no-label-ns")

			allowed, err := reconciler.isNamespaceAllowed(ctx, entry, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeFalse())
		})

		It("anchors the pattern to the full namespace name", func() {
			// "team" must not match "team-blue" since the pattern is anchored.
			server := newServer(&klapv1alpha1.NamespaceSelector{NamePattern: ptr("team")})
			entry := newEntry("team-blue")

			allowed, err := reconciler.isNamespaceAllowed(ctx, entry, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeFalse())
		})

		It("allows an entry whose namespace labels match the selector", func() {
			server := newServer(&klapv1alpha1.NamespaceSelector{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"team": "blue"},
				},
			})
			entry := newEntry("team-blue")

			allowed, err := reconciler.isNamespaceAllowed(ctx, entry, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue())
		})

		It("denies an entry whose namespace labels do not match the selector", func() {
			server := newServer(&klapv1alpha1.NamespaceSelector{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"team": "blue"},
				},
			})
			entry := newEntry("team-red")

			allowed, err := reconciler.isNamespaceAllowed(ctx, entry, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeFalse())
		})
	})
})
