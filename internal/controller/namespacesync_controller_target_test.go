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
