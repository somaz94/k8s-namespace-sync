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
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
)

var _ = Describe("NamespaceSync Controller", func() {
	Context("When creating a NamespaceSync resource", func() {
		It("should sync multiple Secrets and ConfigMaps to new namespaces and clean up properly", func() {
			ctx := context.Background()
			log := logf.FromContext(ctx)

			By("Creating test namespaces")
			log.Info("Creating source namespace")
			sourceNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "source-ns",
				},
			}
			Expect(k8sClient.Create(ctx, sourceNs)).To(Succeed())
			log.Info("Source namespace created", "name", sourceNs.Name)

			targetNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "target-ns",
				},
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
					Data: map[string]string{
						"key": "value",
					},
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
					Data: map[string][]byte{
						"key": []byte("value"),
					},
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
				ObjectMeta: metav1.ObjectMeta{
					Name: "excluded-ns",
				},
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

			// Create a new context with timeout
			cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second*30)
			defer cancel()

			// 1. Delete NamespaceSync first and wait for deletion
			Expect(k8sClient.Delete(cleanupCtx, namespaceSync)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(cleanupCtx, client.ObjectKey{
					Namespace: sourceNs.Name,
					Name:      namespaceSync.Name,
				}, &syncv1.NamespaceSync{})
				return errors.IsNotFound(err)
			}, time.Second*10, time.Second).Should(BeTrue())

			// 2. Wait for synced resources to be deleted from target namespace
			Eventually(func() bool {
				// Check synced secrets
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

				// Check synced configmaps
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

			// 3. Delete namespaces
			Expect(k8sClient.Delete(cleanupCtx, targetNs)).To(Succeed())
			Expect(k8sClient.Delete(cleanupCtx, sourceNs)).To(Succeed())
		})
	})
})

var _ = Describe("NamespaceSync Controller with TargetNamespaces", func() {
	Context("When creating a NamespaceSync resource with targetNamespaces", func() {
		It("should sync resources only to specified target namespaces", func() {
			ctx := context.Background()
			log := logf.FromContext(ctx)
			log.Info("Starting test for targetNamespaces feature")

			By("Creating test namespaces")
			sourceNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "source-ns-target",
				},
			}
			Expect(k8sClient.Create(ctx, sourceNs)).To(Succeed())

			targetNs1 := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "target-ns1",
				},
			}
			Expect(k8sClient.Create(ctx, targetNs1)).To(Succeed())

			targetNs2 := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "target-ns2",
				},
			}
			Expect(k8sClient.Create(ctx, targetNs2)).To(Succeed())

			nonTargetNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "non-target-ns",
				},
			}
			Expect(k8sClient.Create(ctx, nonTargetNs)).To(Succeed())

			By("Creating source ConfigMap and Secret")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "target-test-configmap",
					Namespace: "source-ns-target",
				},
				Data: map[string]string{
					"key": "value",
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "target-test-secret",
					Namespace: "source-ns-target",
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"key": []byte("value"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Creating NamespaceSync resource with targetNamespaces")
			namespaceSync := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sync-target",
					Namespace: "source-ns-target",
					Finalizers: []string{
						"namespacesync.nsync.dev/finalizer",
					},
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace:  "source-ns-target",
					TargetNamespaces: []string{"target-ns1", "target-ns2"},
					SecretName:       []string{"target-test-secret"},
					ConfigMapName:    []string{"target-test-configmap"},
				},
			}
			Expect(k8sClient.Create(ctx, namespaceSync)).To(Succeed())

			By("Verifying resources are synced to target namespaces")
			for _, ns := range []string{"target-ns1", "target-ns2"} {
				Eventually(func() error {
					var targetConfigMap corev1.ConfigMap
					if err := k8sClient.Get(ctx, client.ObjectKey{
						Namespace: ns,
						Name:      "target-test-configmap",
					}, &targetConfigMap); err != nil {
						return err
					}

					var targetSecret corev1.Secret
					if err := k8sClient.Get(ctx, client.ObjectKey{
						Namespace: ns,
						Name:      "target-test-secret",
					}, &targetSecret); err != nil {
						return err
					}
					return nil
				}, time.Second*10, time.Second).Should(Succeed())
			}

			By("Verifying resources are not synced to non-target namespace")
			var nonTargetConfigMap corev1.ConfigMap
			err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: "non-target-ns",
				Name:      "target-test-configmap",
			}, &nonTargetConfigMap)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			By("Cleaning up resources")
			Expect(k8sClient.Delete(ctx, namespaceSync)).To(Succeed())
			Expect(k8sClient.Delete(ctx, sourceNs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, targetNs1)).To(Succeed())
			Expect(k8sClient.Delete(ctx, targetNs2)).To(Succeed())
			Expect(k8sClient.Delete(ctx, nonTargetNs)).To(Succeed())
		})
	})
})

var _ = Describe("NamespaceSync Controller with Resource Filters", func() {
	Context("When creating a NamespaceSync resource with resource filters", func() {
		It("should sync only resources matching the filter patterns", func() {
			ctx := context.Background()
			log := logf.FromContext(ctx)
			log.Info("Starting test for resource filters feature")

			By("Creating test namespaces")
			sourceNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "source-ns-filter",
				},
			}
			Expect(k8sClient.Create(ctx, sourceNs)).To(Succeed())

			targetNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "target-ns-filter",
				},
			}
			Expect(k8sClient.Create(ctx, targetNs)).To(Succeed())

			By("Creating source ConfigMaps and Secrets")
			// Create ConfigMaps with different patterns
			configMaps := []string{"app-config-1", "app-config-2", "other-config"}
			for _, name := range configMaps {
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: "source-ns-filter",
					},
					Data: map[string]string{
						"key": "value",
					},
				}
				Expect(k8sClient.Create(ctx, configMap)).To(Succeed())
			}

			// Create Secrets with different patterns
			secrets := []string{"app-secret-1", "app-secret-2", "other-secret"}
			for _, name := range secrets {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: "source-ns-filter",
					},
					Type: corev1.SecretTypeOpaque,
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			}

			By("Creating NamespaceSync resource with resource filters")
			namespaceSync := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sync-filter",
					Namespace: "source-ns-filter",
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "source-ns-filter",
					ConfigMapName:   configMaps,
					SecretName:      secrets,
					ResourceFilters: &syncv1.ResourceFilters{
						ConfigMaps: &syncv1.ResourceFilter{
							Include: []string{"app-config-*"},
							Exclude: []string{"*-2"},
						},
						Secrets: &syncv1.ResourceFilter{
							Include: []string{"app-secret-*"},
							Exclude: []string{"*-2"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, namespaceSync)).To(Succeed())

			By("Verifying only filtered ConfigMaps are synced")
			Eventually(func() error {
				// Should exist: app-config-1
				var configMap corev1.ConfigMap
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "target-ns-filter",
					Name:      "app-config-1",
				}, &configMap); err != nil {
					return err
				}
				return nil
			}, time.Second*10, time.Second).Should(Succeed())

			By("Verifying excluded ConfigMaps are not synced")
			var configMap corev1.ConfigMap
			err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: "target-ns-filter",
				Name:      "app-config-2",
			}, &configMap)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			err = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: "target-ns-filter",
				Name:      "other-config",
			}, &configMap)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			By("Verifying only filtered Secrets are synced")
			Eventually(func() error {
				// Should exist: app-secret-1
				var secret corev1.Secret
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "target-ns-filter",
					Name:      "app-secret-1",
				}, &secret); err != nil {
					return err
				}
				return nil
			}, time.Second*10, time.Second).Should(Succeed())

			By("Verifying excluded Secrets are not synced")
			var secret corev1.Secret
			err = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: "target-ns-filter",
				Name:      "app-secret-2",
			}, &secret)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			err = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: "target-ns-filter",
				Name:      "other-secret",
			}, &secret)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			By("Cleaning up resources")
			Expect(k8sClient.Delete(ctx, namespaceSync)).To(Succeed())
			Expect(k8sClient.Delete(ctx, sourceNs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, targetNs)).To(Succeed())
		})
	})
})

var _ = Describe("NamespaceSync Controller Resource Deletion", func() {
	Context("When deleting resources in source and target namespaces", func() {
		It("should properly handle resource deletions and re-syncs", func() {
			ctx := context.Background()

			By("Creating test namespaces")
			sourceNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "source-ns-deletion",
				},
			}
			Expect(k8sClient.Create(ctx, sourceNs)).To(Succeed())

			targetNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "target-ns-deletion",
				},
			}
			Expect(k8sClient.Create(ctx, targetNs)).To(Succeed())

			By("Creating source ConfigMap and Secret")
			sourceConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap-deletion",
					Namespace: "source-ns-deletion",
				},
				Data: map[string]string{
					"key": "value",
				},
			}
			Expect(k8sClient.Create(ctx, sourceConfigMap)).To(Succeed())

			sourceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret-deletion",
					Namespace: "source-ns-deletion",
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"key": []byte("value"),
				},
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
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "target-ns-deletion",
					Name:      "test-secret-deletion",
				}, &targetSecret); err != nil {
					return err
				}
				return nil
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
				Data: map[string]string{
					"key": "new-value",
				},
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
