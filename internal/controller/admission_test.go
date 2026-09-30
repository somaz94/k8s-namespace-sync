package controller

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
)

const (
	// No leading slash: envtest joins it onto the webhook URL with one.
	injectLabelPath         = "inject-label"
	injectLabelNamespaceKey = "nsync-test.dev/inject-label"
	injectedLabel           = "injected"
	labelTrue               = "true"
)

// labelInjectingWebhook stands in for a mutating admission policy: it labels every Secret and ConfigMap
// written into a namespace that carries injectLabelNamespaceKey.
func labelInjectingWebhook() *admissionv1.MutatingWebhookConfiguration {
	path := injectLabelPath
	failurePolicy := admissionv1.Fail
	sideEffects := admissionv1.SideEffectClassNone
	return &admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "inject-label"},
		Webhooks: []admissionv1.MutatingWebhook{{
			Name: "inject-label.nsync-test.dev",
			// envtest replaces the service with the URL of the local webhook server.
			ClientConfig: admissionv1.WebhookClientConfig{
				Service: &admissionv1.ServiceReference{Name: "unused", Namespace: "unused", Path: &path},
			},
			Rules: []admissionv1.RuleWithOperations{{
				Operations: []admissionv1.OperationType{admissionv1.Create, admissionv1.Update},
				Rule: admissionv1.Rule{
					APIGroups:   []string{""},
					APIVersions: []string{"v1"},
					Resources:   []string{"secrets", "configmaps"},
				},
			}},
			NamespaceSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{injectLabelNamespaceKey: labelTrue}},
			FailurePolicy:           &failurePolicy,
			SideEffects:             &sideEffects,
			AdmissionReviewVersions: []string{"v1"},
		}},
	}
}

func injectLabel(_ context.Context, req admission.Request) admission.Response {
	var obj unstructured.Unstructured
	if err := obj.UnmarshalJSON(req.Object.Raw); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[injectedLabel] = labelTrue
	obj.SetLabels(labels)
	mutated, err := obj.MarshalJSON()
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, mutated)
}

var _ = Describe("Mutating admission on synced copies", func() {
	It("leaves a copy alone when admission re-adds the labels the sync strips", func(ctx SpecContext) {
		By("creating a target namespace the label-injecting webhook covers")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "adm-src"}})).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   "adm-tgt",
			Labels: map[string]string{injectLabelNamespaceKey: labelTrue},
		}})).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "adm-secret", Namespace: "adm-src"},
			Data:       map[string][]byte{"key": []byte("value")},
		})).To(Succeed())

		namespaceSync := &syncv1.NamespaceSync{
			ObjectMeta: metav1.ObjectMeta{Name: "adm-sync", Namespace: "adm-src"},
			Spec: syncv1.NamespaceSyncSpec{
				SourceNamespace:  "adm-src",
				TargetNamespaces: []string{"adm-tgt"},
				SecretName:       []string{"adm-secret"},
			},
		}
		Expect(k8sClient.Create(ctx, namespaceSync)).To(Succeed())

		copyKey := client.ObjectKey{Namespace: "adm-tgt", Name: "adm-secret"}
		var synced corev1.Secret
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, copyKey, &synced)).To(Succeed())
			g.Expect(synced.Labels).To(HaveKeyWithValue(injectedLabel, labelTrue))
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		var (
			lastSyncTime    metav1.Time
			resourceVersion string
		)
		// Both stamps have one-second resolution, so a write in the same second would look like a no-op. Both are
		// re-read on every poll, since the reconcile the copy's creation triggers can still move them.
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, copyKey, &synced)).To(Succeed())
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(namespaceSync), namespaceSync)).To(Succeed())
			g.Expect(namespaceSync.Status.LastSyncTime.IsZero()).To(BeFalse())
			stamp, err := time.Parse(time.RFC3339, synced.Annotations[AnnotationLastSync])
			g.Expect(err).NotTo(HaveOccurred())
			lastSyncTime, resourceVersion = namespaceSync.Status.LastSyncTime, synced.ResourceVersion
			g.Expect(time.Now().After(stamp.Add(time.Second))).To(BeTrue())
			g.Expect(time.Now().After(lastSyncTime.Add(time.Second))).To(BeTrue())
		}, 10*time.Second, 50*time.Millisecond).Should(Succeed())

		By("forcing a resync, which strips the injected label")
		Eventually(func() error {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(namespaceSync), namespaceSync); err != nil {
				return err
			}
			metav1.SetMetaDataAnnotation(&namespaceSync.ObjectMeta, "nsync-test.dev/resync", "1")
			return k8sClient.Update(ctx, namespaceSync)
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(namespaceSync), namespaceSync)).To(Succeed())
			g.Expect(namespaceSync.Status.LastSyncTime.After(lastSyncTime.Time)).To(BeTrue())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		Expect(k8sClient.Get(ctx, copyKey, &synced)).To(Succeed())
		Expect(synced.ResourceVersion).To(Equal(resourceVersion), "the resync should have been a no-op for the apiserver")
		Expect(synced.Labels).To(HaveKeyWithValue(injectedLabel, labelTrue))

		Expect(k8sClient.Delete(ctx, namespaceSync)).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(namespaceSync), &syncv1.NamespaceSync{}))
		}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())
	})
})
