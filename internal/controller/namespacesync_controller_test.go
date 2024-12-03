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
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
)

var _ = Describe("NamespaceSync Controller", func() {
	Context("When creating a NamespaceSync resource", func() {
		It("should sync Secret and ConfigMap to new namespaces", func() {
			ctx := context.Background()

			// Create default namespace if it doesn't exist
			defaultNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			err := k8sClient.Create(ctx, defaultNs)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			// Create source Secret
			sourceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"test-key": []byte("test-value"),
				},
			}
			Expect(k8sClient.Create(ctx, sourceSecret)).To(Succeed())

			// Create source ConfigMap
			sourceConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "default",
				},
				Data: map[string]string{
					"test-key": "test-value",
				},
			}
			Expect(k8sClient.Create(ctx, sourceConfigMap)).To(Succeed())

			// Create a new namespace
			newNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace",
				},
			}
			Expect(k8sClient.Create(ctx, newNamespace)).To(Succeed())

			// Start the controller
			k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: scheme.Scheme,
			})
			Expect(err).ToNot(HaveOccurred())

			controller := &NamespaceSyncReconciler{
				Client: k8sManager.GetClient(),
				Scheme: k8sManager.GetScheme(),
			}
			Expect(controller.SetupWithManager(k8sManager)).To(Succeed())

			go func() {
				err = k8sManager.Start(ctx)
				Expect(err).ToNot(HaveOccurred())
			}()

			// Create a NamespaceSync resource
			namespaceSync := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sync",
					Namespace: "default",
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "default",
					SecretName:      "test-secret",
					ConfigMapName:   "test-configmap",
				},
			}
			Expect(k8sClient.Create(ctx, namespaceSync)).To(Succeed())

			// Check if the Secret is synced
			var secret corev1.Secret
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "test-namespace",
					Name:      "test-secret",
				}, &secret)
			}, time.Second*30, time.Second).Should(Succeed())

			// Check if the ConfigMap is synced
			var configMap corev1.ConfigMap
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{
					Namespace: "test-namespace",
					Name:      "test-configmap",
				}, &configMap)
			}, time.Second*30, time.Second).Should(Succeed())

			// Cleanup
			Expect(k8sClient.Delete(ctx, sourceSecret)).To(Succeed())
			Expect(k8sClient.Delete(ctx, sourceConfigMap)).To(Succeed())
			Expect(k8sClient.Delete(ctx, namespaceSync)).To(Succeed())
			Expect(k8sClient.Delete(ctx, newNamespace)).To(Succeed())
		})
	})
})
