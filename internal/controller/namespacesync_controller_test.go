/*
Copyright 2024.

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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
)

var _ = Describe("NamespaceSync Controller", func() {
	Context("When creating a NamespaceSync resource", func() {
		It("should sync multiple Secrets and ConfigMaps to new namespaces and clean up properly", func() {
			ctx := context.Background()

			By("Creating test namespaces")
			sourceNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "source-ns"},
			}
			Expect(k8sClient.Create(ctx, sourceNs)).To(Succeed())

			targetNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "target-ns"},
			}
			Expect(k8sClient.Create(ctx, targetNs)).To(Succeed())

			By("Creating source ConfigMaps and Secrets")
			sourceConfigMaps := []string{"configmap1", "configmap2"}
			sourceSecrets := []string{"secret1", "secret2"}

			for _, name := range sourceConfigMaps {
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: "source-ns",
					},
					Data: map[string]string{"key": "value"},
				}
				Expect(k8sClient.Create(ctx, configMap)).To(Succeed())
			}

			for _, name := range sourceSecrets {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: "source-ns",
					},
					Type: corev1.SecretTypeOpaque,
					Data: map[string][]byte{"key": []byte("value")},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			}

			By("Creating NamespaceSync resource")
			namespaceSync := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sync",
					Namespace: "source-ns",
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "source-ns",
					SecretName:      []string{"secret1", "secret2"},
					ConfigMapName:   []string{"configmap1", "configmap2"},
					Exclude:         []string{"excluded-ns"},
				},
			}
			Expect(k8sClient.Create(ctx, namespaceSync)).To(Succeed())

			By("Verifying ConfigMap sync")
			for _, configMapName := range namespaceSync.Spec.ConfigMapName {
				var targetConfigMap corev1.ConfigMap
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKey{
						Namespace: "target-ns",
						Name:      configMapName,
					}, &targetConfigMap)
				}, time.Second*10, time.Second).Should(Succeed())

				Expect(targetConfigMap.Data).To(Equal(map[string]string{"key": "value"}))
			}

			By("Creating excluded namespace")
			excludedNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "excluded-ns"},
			}
			Expect(k8sClient.Create(ctx, excludedNs)).To(Succeed())

			By("Updating NamespaceSync with excluded namespace")
			Eventually(func() error {
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "source-ns",
					Name:      "test-sync",
				}, namespaceSync); err != nil {
					return err
				}
				namespaceSync.Spec.Exclude = []string{"excluded-ns"}
				return k8sClient.Update(ctx, namespaceSync)
			}, time.Second*10, time.Second).Should(Succeed())

			By("Verifying resources are not synced to excluded namespace")
			var err error
			excludedSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: "excluded-ns",
				Name:      "test-secret",
			}, excludedSecret)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "Secret should not exist in excluded namespace")

			excludedConfigMap := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: "excluded-ns",
				Name:      "test-configmap",
			}, excludedConfigMap)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "ConfigMap should not exist in excluded namespace")

			By("Cleaning up resources")
			cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second*30)
			defer cancel()

			Expect(k8sClient.Delete(cleanupCtx, namespaceSync)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(cleanupCtx, client.ObjectKey{
					Namespace: sourceNs.Name,
					Name:      namespaceSync.Name,
				}, &syncv1.NamespaceSync{})
				return errors.IsNotFound(err)
			}, time.Second*10, time.Second).Should(BeTrue())

			Eventually(func() bool {
				for _, secretName := range namespaceSync.Spec.SecretName {
					secret := &corev1.Secret{}
					err := k8sClient.Get(cleanupCtx, client.ObjectKey{
						Namespace: targetNs.Name,
						Name:      secretName,
					}, secret)
					if !errors.IsNotFound(err) {
						return false
					}
				}
				for _, configMapName := range namespaceSync.Spec.ConfigMapName {
					configMap := &corev1.ConfigMap{}
					err := k8sClient.Get(cleanupCtx, client.ObjectKey{
						Namespace: targetNs.Name,
						Name:      configMapName,
					}, configMap)
					if !errors.IsNotFound(err) {
						return false
					}
				}
				return true
			}, time.Second*10, time.Second).Should(BeTrue(), "Synced resources should be deleted from target namespace")

			Expect(k8sClient.Delete(cleanupCtx, targetNs)).To(Succeed())
			Expect(k8sClient.Delete(cleanupCtx, sourceNs)).To(Succeed())
		})
	})
})

var _ = Describe("NamespaceSync Controller Resource Deletion", func() {
	Context("When deleting resources in source and target namespaces", func() {
		It("should properly handle resource deletions and re-syncs", func() {
			ctx := context.Background()

			By("Creating test namespaces")
			sourceNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "source-ns-deletion"},
			}
			Expect(k8sClient.Create(ctx, sourceNs)).To(Succeed())

			targetNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "target-ns-deletion"},
			}
			Expect(k8sClient.Create(ctx, targetNs)).To(Succeed())

			By("Creating source ConfigMap and Secret")
			sourceConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap-deletion",
					Namespace: "source-ns-deletion",
				},
				Data: map[string]string{"key": "value"},
			}
			Expect(k8sClient.Create(ctx, sourceConfigMap)).To(Succeed())

			sourceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret-deletion",
					Namespace: "source-ns-deletion",
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{"key": []byte("value")},
			}
			Expect(k8sClient.Create(ctx, sourceSecret)).To(Succeed())

			By("Creating NamespaceSync resource")
			namespaceSync := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sync-deletion",
					Namespace: "source-ns-deletion",
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "source-ns-deletion",
					SecretName:      []string{"test-secret-deletion"},
					ConfigMapName:   []string{"test-configmap-deletion"},
				},
			}
			Expect(k8sClient.Create(ctx, namespaceSync)).To(Succeed())

			By("Verifying initial sync to target namespace")
			Eventually(func() error {
				var targetConfigMap corev1.ConfigMap
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "target-ns-deletion",
					Name:      "test-configmap-deletion",
				}, &targetConfigMap); err != nil {
					return err
				}
				var targetSecret corev1.Secret
				return k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "target-ns-deletion",
					Name:      "test-secret-deletion",
				}, &targetSecret)
			}, time.Second*10, time.Second).Should(Succeed())

			By("Deleting source ConfigMap")
			Expect(k8sClient.Delete(ctx, sourceConfigMap)).To(Succeed())

			By("Verifying ConfigMap is deleted from target namespace")
			Eventually(func() bool {
				var targetConfigMap corev1.ConfigMap
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "target-ns-deletion",
					Name:      "test-configmap-deletion",
				}, &targetConfigMap)
				return errors.IsNotFound(err)
			}, time.Second*10, time.Second).Should(BeTrue())

			By("Recreating source ConfigMap")
			sourceConfigMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap-deletion",
					Namespace: "source-ns-deletion",
				},
				Data: map[string]string{"key": "new-value"},
			}
			Expect(k8sClient.Create(ctx, sourceConfigMap)).To(Succeed())

			By("Verifying ConfigMap is re-synced to target namespace")
			Eventually(func() error {
				var targetConfigMap corev1.ConfigMap
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "target-ns-deletion",
					Name:      "test-configmap-deletion",
				}, &targetConfigMap); err != nil {
					return err
				}
				Expect(targetConfigMap.Data["key"]).To(Equal("new-value"))
				return nil
			}, time.Second*10, time.Second).Should(Succeed())

			By("Deleting ConfigMap from target namespace")
			targetConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap-deletion",
					Namespace: "target-ns-deletion",
				},
			}
			Expect(k8sClient.Delete(ctx, targetConfigMap)).To(Succeed())

			By("Verifying ConfigMap is re-synced from source namespace")
			Eventually(func() error {
				var resyncdConfigMap corev1.ConfigMap
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "target-ns-deletion",
					Name:      "test-configmap-deletion",
				}, &resyncdConfigMap); err != nil {
					return err
				}
				Expect(resyncdConfigMap.Data["key"]).To(Equal("new-value"))
				return nil
			}, time.Second*10, time.Second).Should(Succeed())

			By("Deleting source Secret")
			Expect(k8sClient.Delete(ctx, sourceSecret)).To(Succeed())

			By("Verifying Secret is deleted from target namespace")
			Eventually(func() bool {
				var targetSecret corev1.Secret
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "target-ns-deletion",
					Name:      "test-secret-deletion",
				}, &targetSecret)
				return errors.IsNotFound(err)
			}, time.Second*10, time.Second).Should(BeTrue())

			By("Cleaning up resources")
			Expect(k8sClient.Delete(ctx, namespaceSync)).To(Succeed())
			Expect(k8sClient.Delete(ctx, sourceNs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, targetNs)).To(Succeed())
		})
	})
})

var _ = Describe("NamespaceSync Validation", func() {
	Context("When creating an invalid NamespaceSync resource", func() {
		It("should fail validation with empty sourceNamespace", func() {
			ctx := context.Background()

			By("Creating namespace for invalid sync")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "validation-ns"},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			By("Creating NamespaceSync with empty sourceNamespace")
			invalidSync := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-empty-source",
					Namespace: "validation-ns",
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "",
					SecretName:      []string{"some-secret"},
				},
			}
			Expect(k8sClient.Create(ctx, invalidSync)).To(Succeed())

			By("Verifying status reflects validation error")
			Eventually(func() bool {
				var ns syncv1.NamespaceSync
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "validation-ns",
					Name:      "invalid-empty-source",
				}, &ns); err != nil {
					return false
				}
				if ns.Status.FailedNamespaces == nil {
					return false
				}
				_, hasValidation := ns.Status.FailedNamespaces["validation"]
				return hasValidation
			}, time.Second*10, time.Second).Should(BeTrue(), "Should have validation error in status")

			Expect(k8sClient.Delete(ctx, invalidSync)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
		})

		It("should fail validation with no secrets or configmaps specified", func() {
			ctx := context.Background()

			By("Creating namespace for invalid sync")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "validation-ns2"},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			By("Creating NamespaceSync with no resources")
			invalidSync := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-no-resources",
					Namespace: "validation-ns2",
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "validation-ns2",
				},
			}
			Expect(k8sClient.Create(ctx, invalidSync)).To(Succeed())

			By("Verifying status reflects validation error")
			Eventually(func() bool {
				var ns syncv1.NamespaceSync
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "validation-ns2",
					Name:      "invalid-no-resources",
				}, &ns); err != nil {
					return false
				}
				if ns.Status.FailedNamespaces == nil {
					return false
				}
				_, hasValidation := ns.Status.FailedNamespaces["validation"]
				return hasValidation
			}, time.Second*10, time.Second).Should(BeTrue(), "Should have validation error in status")

			Expect(k8sClient.Delete(ctx, invalidSync)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
		})
	})
})

var _ = Describe("NamespaceSync Target Namespaces", func() {
	Context("When targetNamespaces is specified", func() {
		It("should only sync to specified target namespaces", func() {
			ctx := context.Background()

			By("Creating namespaces")
			sourceNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "target-src-ns"},
			}
			Expect(k8sClient.Create(ctx, sourceNs)).To(Succeed())

			targetNs1 := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "target-allowed-ns"},
			}
			Expect(k8sClient.Create(ctx, targetNs1)).To(Succeed())

			targetNs2 := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "target-denied-ns"},
			}
			Expect(k8sClient.Create(ctx, targetNs2)).To(Succeed())

			By("Creating source configmap")
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "target-test-cm",
					Namespace: "target-src-ns",
				},
				Data: map[string]string{"key": "value"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("Creating NamespaceSync with targetNamespaces")
			ns := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-target-ns",
					Namespace: "target-src-ns",
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace:  "target-src-ns",
					TargetNamespaces: []string{"target-allowed-ns"},
					ConfigMapName:    []string{"target-test-cm"},
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			By("Verifying configmap synced to allowed namespace")
			Eventually(func() error {
				var targetCm corev1.ConfigMap
				return k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "target-allowed-ns",
					Name:      "target-test-cm",
				}, &targetCm)
			}, time.Second*10, time.Second).Should(Succeed())

			By("Verifying configmap NOT synced to denied namespace")
			Consistently(func() bool {
				var targetCm corev1.ConfigMap
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "target-denied-ns",
					Name:      "target-test-cm",
				}, &targetCm)
				return errors.IsNotFound(err)
			}, time.Second*3, time.Millisecond*500).Should(BeTrue())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "target-src-ns", Name: "test-target-ns",
				}, &syncv1.NamespaceSync{})
				return errors.IsNotFound(err)
			}, time.Second*10, time.Second).Should(BeTrue())
			Expect(k8sClient.Delete(ctx, sourceNs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, targetNs1)).To(Succeed())
			Expect(k8sClient.Delete(ctx, targetNs2)).To(Succeed())
		})
	})
})

var _ = Describe("NamespaceSync Resource Filters", func() {
	Context("When resourceFilters are specified", func() {
		It("should apply include/exclude filters to resources", func() {
			ctx := context.Background()

			By("Creating namespaces")
			sourceNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "filter-src-ns"},
			}
			Expect(k8sClient.Create(ctx, sourceNs)).To(Succeed())

			targetNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "filter-target-ns"},
			}
			Expect(k8sClient.Create(ctx, targetNs)).To(Succeed())

			By("Creating source resources")
			cm1 := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "app-config", Namespace: "filter-src-ns",
				},
				Data: map[string]string{"key": "value"},
			}
			Expect(k8sClient.Create(ctx, cm1)).To(Succeed())

			cm2 := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "app-config-backup", Namespace: "filter-src-ns",
				},
				Data: map[string]string{"key": "value"},
			}
			Expect(k8sClient.Create(ctx, cm2)).To(Succeed())

			secret1 := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "db-secret", Namespace: "filter-src-ns",
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{"key": []byte("value")},
			}
			Expect(k8sClient.Create(ctx, secret1)).To(Succeed())

			secret2 := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "db-secret-backup", Namespace: "filter-src-ns",
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{"key": []byte("value")},
			}
			Expect(k8sClient.Create(ctx, secret2)).To(Succeed())

			By("Creating NamespaceSync with resource filters (exclude *-backup)")
			ns := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-filter",
					Namespace: "filter-src-ns",
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "filter-src-ns",
					SecretName:      []string{"db-secret", "db-secret-backup"},
					ConfigMapName:   []string{"app-config", "app-config-backup"},
					ResourceFilters: &syncv1.ResourceFilters{
						Secrets: &syncv1.ResourceFilter{
							Exclude: []string{"*-backup"},
						},
						ConfigMaps: &syncv1.ResourceFilter{
							Exclude: []string{"*-backup"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			By("Verifying non-backup resources are synced")
			Eventually(func() error {
				var cm corev1.ConfigMap
				return k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "filter-target-ns", Name: "app-config",
				}, &cm)
			}, time.Second*10, time.Second).Should(Succeed())

			Eventually(func() error {
				var s corev1.Secret
				return k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "filter-target-ns", Name: "db-secret",
				}, &s)
			}, time.Second*10, time.Second).Should(Succeed())

			By("Verifying backup resources are NOT synced")
			Consistently(func() bool {
				var cm corev1.ConfigMap
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "filter-target-ns", Name: "app-config-backup",
				}, &cm)
				return errors.IsNotFound(err)
			}, time.Second*3, time.Millisecond*500).Should(BeTrue())

			Consistently(func() bool {
				var s corev1.Secret
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "filter-target-ns", Name: "db-secret-backup",
				}, &s)
				return errors.IsNotFound(err)
			}, time.Second*3, time.Millisecond*500).Should(BeTrue())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "filter-src-ns", Name: "test-filter",
				}, &syncv1.NamespaceSync{})
				return errors.IsNotFound(err)
			}, time.Second*10, time.Second).Should(BeTrue())
			Expect(k8sClient.Delete(ctx, sourceNs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, targetNs)).To(Succeed())
		})

		It("should apply include filters to select specific resources", func() {
			ctx := context.Background()

			By("Creating namespaces")
			sourceNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "incl-src-ns"},
			}
			Expect(k8sClient.Create(ctx, sourceNs)).To(Succeed())

			targetNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "incl-target-ns"},
			}
			Expect(k8sClient.Create(ctx, targetNs)).To(Succeed())

			By("Creating source resources")
			cm1 := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "prod-config", Namespace: "incl-src-ns",
				},
				Data: map[string]string{"env": "prod"},
			}
			Expect(k8sClient.Create(ctx, cm1)).To(Succeed())

			cm2 := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dev-config", Namespace: "incl-src-ns",
				},
				Data: map[string]string{"env": "dev"},
			}
			Expect(k8sClient.Create(ctx, cm2)).To(Succeed())

			By("Creating NamespaceSync with include filter (only prod-*)")
			ns := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-include-filter",
					Namespace: "incl-src-ns",
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "incl-src-ns",
					ConfigMapName:   []string{"prod-config", "dev-config"},
					ResourceFilters: &syncv1.ResourceFilters{
						ConfigMaps: &syncv1.ResourceFilter{
							Include: []string{"prod-*"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			By("Verifying prod-config is synced")
			Eventually(func() error {
				var cm corev1.ConfigMap
				return k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "incl-target-ns", Name: "prod-config",
				}, &cm)
			}, time.Second*10, time.Second).Should(Succeed())

			By("Verifying dev-config is NOT synced")
			Consistently(func() bool {
				var cm corev1.ConfigMap
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "incl-target-ns", Name: "dev-config",
				}, &cm)
				return errors.IsNotFound(err)
			}, time.Second*3, time.Millisecond*500).Should(BeTrue())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "incl-src-ns", Name: "test-include-filter",
				}, &syncv1.NamespaceSync{})
				return errors.IsNotFound(err)
			}, time.Second*10, time.Second).Should(BeTrue())
			Expect(k8sClient.Delete(ctx, sourceNs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, targetNs)).To(Succeed())
		})
	})
})

var _ = Describe("NamespaceSync Labels and Annotations", func() {
	Context("When syncing resources with labels and annotations", func() {
		It("should copy labels and annotations but exclude kubernetes.io prefixed ones", func() {
			ctx := context.Background()

			By("Creating namespaces")
			sourceNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "label-src-ns"},
			}
			Expect(k8sClient.Create(ctx, sourceNs)).To(Succeed())

			targetNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "label-target-ns"},
			}
			Expect(k8sClient.Create(ctx, targetNs)).To(Succeed())

			By("Creating source configmap with labels and annotations")
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labeled-cm",
					Namespace: "label-src-ns",
					Labels: map[string]string{
						"app":                      "myapp",
						"version":                  "v1",
						"kubernetes.io/managed-by": "helm",
					},
					Annotations: map[string]string{
						"custom-annotation":         "custom-value",
						"kubernetes.io/description": "should-not-copy",
					},
				},
				Data: map[string]string{"key": "value"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("Creating NamespaceSync")
			ns := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-labels",
					Namespace: "label-src-ns",
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "label-src-ns",
					ConfigMapName:   []string{"labeled-cm"},
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			By("Verifying synced configmap has correct labels/annotations")
			Eventually(func() error {
				var targetCm corev1.ConfigMap
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "label-target-ns", Name: "labeled-cm",
				}, &targetCm); err != nil {
					return err
				}

				// Custom labels should be copied
				Expect(targetCm.Labels["app"]).To(Equal("myapp"))
				Expect(targetCm.Labels["version"]).To(Equal("v1"))

				// kubernetes.io/ labels should NOT be copied
				_, hasK8sLabel := targetCm.Labels["kubernetes.io/managed-by"]
				Expect(hasK8sLabel).To(BeFalse(), "kubernetes.io/ label should not be copied")

				// Custom annotations should be copied
				Expect(targetCm.Annotations["custom-annotation"]).To(Equal("custom-value"))

				// kubernetes.io/ annotations should NOT be copied
				_, hasK8sAnnotation := targetCm.Annotations["kubernetes.io/description"]
				Expect(hasK8sAnnotation).To(BeFalse(), "kubernetes.io/ annotation should not be copied")

				// Sync metadata annotations should exist
				Expect(targetCm.Annotations).To(HaveKey("namespacesync.nsync.dev/source-namespace"))
				Expect(targetCm.Annotations).To(HaveKey("namespacesync.nsync.dev/source-name"))
				Expect(targetCm.Annotations).To(HaveKey("namespacesync.nsync.dev/last-sync"))

				return nil
			}, time.Second*10, time.Second).Should(Succeed())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "label-src-ns", Name: "test-labels",
				}, &syncv1.NamespaceSync{})
				return errors.IsNotFound(err)
			}, time.Second*10, time.Second).Should(BeTrue())
			Expect(k8sClient.Delete(ctx, sourceNs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, targetNs)).To(Succeed())
		})
	})
})

var _ = Describe("NamespaceSync Status Updates", func() {
	Context("When syncing is successful", func() {
		It("should update status with synced namespaces and Ready condition", func() {
			ctx := context.Background()

			By("Creating namespaces")
			sourceNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "status-src-ns"},
			}
			Expect(k8sClient.Create(ctx, sourceNs)).To(Succeed())

			targetNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "status-target-ns"},
			}
			Expect(k8sClient.Create(ctx, targetNs)).To(Succeed())

			By("Creating source secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "status-secret", Namespace: "status-src-ns",
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{"key": []byte("value")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Creating NamespaceSync with targetNamespaces to limit scope")
			ns := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-status",
					Namespace: "status-src-ns",
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace:  "status-src-ns",
					TargetNamespaces: []string{"status-target-ns"},
					SecretName:       []string{"status-secret"},
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			By("Verifying secret is synced first")
			Eventually(func() error {
				var s corev1.Secret
				return k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "status-target-ns", Name: "status-secret",
				}, &s)
			}, time.Second*15, time.Second).Should(Succeed())

			By("Verifying status is updated with Ready condition and synced namespaces")
			Eventually(func() bool {
				var sync syncv1.NamespaceSync
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "status-src-ns", Name: "test-status",
				}, &sync); err != nil {
					return false
				}

				hasReady := false
				for _, c := range sync.Status.Conditions {
					if c.Type == "Ready" && c.Status == metav1.ConditionTrue {
						hasReady = true
						break
					}
				}
				hasSynced := false
				for _, ns := range sync.Status.SyncedNamespaces {
					if ns == "status-target-ns" {
						hasSynced = true
						break
					}
				}
				return hasReady && hasSynced
			}, time.Second*30, time.Second).Should(BeTrue(), "Should have Ready=True and status-target-ns in synced list")

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "status-src-ns", Name: "test-status",
				}, &syncv1.NamespaceSync{})
				return errors.IsNotFound(err)
			}, time.Second*10, time.Second).Should(BeTrue())
			Expect(k8sClient.Delete(ctx, sourceNs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, targetNs)).To(Succeed())
		})
	})
})

var _ = Describe("NamespaceSync Dynamic Namespace", func() {
	Context("When a new namespace is created after NamespaceSync", func() {
		It("should sync resources to the newly created namespace", func() {
			ctx := context.Background()

			By("Creating source namespace and resources")
			sourceNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "dynamic-src-ns"},
			}
			Expect(k8sClient.Create(ctx, sourceNs)).To(Succeed())

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dynamic-cm", Namespace: "dynamic-src-ns",
				},
				Data: map[string]string{"key": "value"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("Creating NamespaceSync")
			ns := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dynamic",
					Namespace: "dynamic-src-ns",
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "dynamic-src-ns",
					ConfigMapName:   []string{"dynamic-cm"},
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			By("Waiting for initial reconcile")
			Eventually(func() bool {
				var sync syncv1.NamespaceSync
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "dynamic-src-ns", Name: "test-dynamic",
				}, &sync); err != nil {
					return false
				}
				return len(sync.Status.Conditions) > 0
			}, time.Second*10, time.Second).Should(BeTrue())

			By("Creating a new namespace after NamespaceSync exists")
			newNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "dynamic-new-ns"},
			}
			Expect(k8sClient.Create(ctx, newNs)).To(Succeed())

			By("Verifying resources are synced to the new namespace")
			Eventually(func() error {
				var targetCm corev1.ConfigMap
				return k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "dynamic-new-ns", Name: "dynamic-cm",
				}, &targetCm)
			}, time.Second*15, time.Second).Should(Succeed())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "dynamic-src-ns", Name: "test-dynamic",
				}, &syncv1.NamespaceSync{})
				return errors.IsNotFound(err)
			}, time.Second*10, time.Second).Should(BeTrue())
			Expect(k8sClient.Delete(ctx, sourceNs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, newNs)).To(Succeed())
		})
	})
})

var _ = Describe("NamespaceSync Source Update", func() {
	Context("When source resource is updated", func() {
		It("should propagate changes to target namespaces", func() {
			ctx := context.Background()

			By("Creating namespaces")
			sourceNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "update-src-ns"},
			}
			Expect(k8sClient.Create(ctx, sourceNs)).To(Succeed())

			targetNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "update-target-ns"},
			}
			Expect(k8sClient.Create(ctx, targetNs)).To(Succeed())

			By("Creating source secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "update-secret", Namespace: "update-src-ns",
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{"password": []byte("old-password")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Creating NamespaceSync")
			ns := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-update",
					Namespace: "update-src-ns",
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "update-src-ns",
					SecretName:      []string{"update-secret"},
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			By("Verifying initial sync")
			Eventually(func() error {
				var s corev1.Secret
				return k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "update-target-ns", Name: "update-secret",
				}, &s)
			}, time.Second*10, time.Second).Should(Succeed())

			By("Updating source secret")
			Eventually(func() error {
				var s corev1.Secret
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "update-src-ns", Name: "update-secret",
				}, &s); err != nil {
					return err
				}
				s.Data["password"] = []byte("new-password")
				return k8sClient.Update(ctx, &s)
			}, time.Second*10, time.Second).Should(Succeed())

			By("Verifying updated data is synced to target namespace")
			Eventually(func() bool {
				var s corev1.Secret
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "update-target-ns", Name: "update-secret",
				}, &s); err != nil {
					return false
				}
				return string(s.Data["password"]) == "new-password"
			}, time.Second*15, time.Second).Should(BeTrue())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "update-src-ns", Name: "test-update",
				}, &syncv1.NamespaceSync{})
				return errors.IsNotFound(err)
			}, time.Second*10, time.Second).Should(BeTrue())
			Expect(k8sClient.Delete(ctx, sourceNs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, targetNs)).To(Succeed())
		})
	})
})

var _ = Describe("NamespaceSync BinaryData ConfigMap", func() {
	Context("When syncing ConfigMap with BinaryData", func() {
		It("should sync BinaryData correctly", func() {
			ctx := context.Background()

			By("Creating namespaces")
			sourceNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "binary-src-ns"},
			}
			Expect(k8sClient.Create(ctx, sourceNs)).To(Succeed())

			targetNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "binary-target-ns"},
			}
			Expect(k8sClient.Create(ctx, targetNs)).To(Succeed())

			By("Creating source configmap with BinaryData")
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "binary-cm", Namespace: "binary-src-ns",
				},
				Data:       map[string]string{"text": "hello"},
				BinaryData: map[string][]byte{"binary": {0x00, 0x01, 0x02}},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("Creating NamespaceSync")
			ns := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-binary",
					Namespace: "binary-src-ns",
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "binary-src-ns",
					ConfigMapName:   []string{"binary-cm"},
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			By("Verifying BinaryData is synced")
			Eventually(func() error {
				var targetCm corev1.ConfigMap
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "binary-target-ns", Name: "binary-cm",
				}, &targetCm); err != nil {
					return err
				}
				Expect(targetCm.Data["text"]).To(Equal("hello"))
				Expect(targetCm.BinaryData["binary"]).To(Equal([]byte{0x00, 0x01, 0x02}))
				return nil
			}, time.Second*10, time.Second).Should(Succeed())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "binary-src-ns", Name: "test-binary",
				}, &syncv1.NamespaceSync{})
				return errors.IsNotFound(err)
			}, time.Second*10, time.Second).Should(BeTrue())
			Expect(k8sClient.Delete(ctx, sourceNs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, targetNs)).To(Succeed())
		})
	})
})
