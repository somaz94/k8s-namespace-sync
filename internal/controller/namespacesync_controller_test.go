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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
)

var _ = Describe("NamespaceSync Controller", func() {
	Context("When creating a NamespaceSync resource", func() {
		It("should sync Secret and ConfigMap to new namespaces", func() {
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

			By("Creating source Secret")
			sourceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "source-ns",
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"test-key": []byte("test-value"),
				},
			}
			Expect(k8sClient.Create(ctx, sourceSecret)).To(Succeed())

			By("Creating source ConfigMap")
			sourceConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "source-ns",
				},
				Data: map[string]string{
					"test-key": "test-value",
				},
			}
			Expect(k8sClient.Create(ctx, sourceConfigMap)).To(Succeed())

			By("Creating NamespaceSync resource")
			namespaceSync := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sync",
					Namespace: "source-ns",
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "source-ns",
					SecretName:      "test-secret",
					ConfigMapName:   "test-configmap",
				},
			}
			Expect(k8sClient.Create(ctx, namespaceSync)).To(Succeed())

			By("Verifying Secret sync")
			var targetSecret corev1.Secret
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "target-ns",
					Name:      "test-secret",
				}, &targetSecret)
				if err != nil {
					log.Error(err, "Failed to get target secret")
					return err
				}
				log.Info("Found target secret",
					"namespace", targetSecret.Namespace,
					"name", targetSecret.Name,
					"data", targetSecret.Data)
				return nil
			}, time.Second*10, time.Second).Should(Succeed())

			Expect(targetSecret.Data).To(Equal(sourceSecret.Data))

			By("Verifying ConfigMap sync")
			var targetConfigMap corev1.ConfigMap
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "target-ns",
					Name:      "test-configmap",
				}, &targetConfigMap)
			}, time.Second*10, time.Second).Should(Succeed())

			Expect(targetConfigMap.Data).To(Equal(sourceConfigMap.Data))

			By("Updating source Secret")
			sourceSecret.Data["test-key"] = []byte("updated-value")
			Expect(k8sClient.Update(ctx, sourceSecret)).To(Succeed())

			By("Verifying Secret update sync")
			Eventually(func() []byte {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "target-ns",
					Name:      "test-secret",
				}, &targetSecret)
				if err != nil {
					return nil
				}
				return targetSecret.Data["test-key"]
			}, time.Second*10, time.Second).Should(Equal([]byte("updated-value")))

			By("Cleaning up resources")
			Expect(k8sClient.Delete(ctx, namespaceSync)).To(Succeed())
			Expect(k8sClient.Delete(ctx, sourceNs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, targetNs)).To(Succeed())
		})
	})
})
