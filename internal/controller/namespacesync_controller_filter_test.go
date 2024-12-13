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
