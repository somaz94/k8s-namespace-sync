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
