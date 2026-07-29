package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
)

// These specs exercise the CRD schema itself (field constraints and CEL
// x-kubernetes-validations) against the envtest apiserver. They mirror the
// checks in validateNamespaceSync, promoting them from reconcile time to
// admission time so an invalid resource is rejected by kubectl apply instead
// of being accepted and then failing on every reconcile.
var _ = Describe("NamespaceSync CRD validation", func() {
	ctx := context.Background()
	var namespace string

	BeforeEach(func() {
		namespace = fmt.Sprintf("crd-validation-%d", time.Now().UnixNano())
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		})).To(Succeed())
	})

	// created tracks the resources that were admitted, so AfterEach can remove
	// them. A NamespaceSync left behind keeps reconciling cluster-wide and would
	// interfere with the sync specs in the other files.
	var created []*syncv1.NamespaceSync

	AfterEach(func() {
		for _, ns := range created {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ns))).To(Succeed())
		}
		created = nil
	})

	// newSync returns a spec that satisfies every CRD rule, so each spec below
	// can mutate exactly the one field under test. targetNamespaces is pinned to
	// this spec's own namespace and the resource names are unique to this file,
	// so an admitted resource cannot sync over another test's fixtures.
	newSync := func(name string) *syncv1.NamespaceSync {
		return &syncv1.NamespaceSync{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: syncv1.NamespaceSyncSpec{
				SourceNamespace:  "default",
				TargetNamespaces: []string{namespace},
				ConfigMapName:    []string{"crd-validation-config"},
			},
		}
	}

	// createOK creates a resource that is expected to pass admission and
	// registers it for cleanup.
	createOK := func(ns *syncv1.NamespaceSync) {
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		created = append(created, ns)
	}

	Context("spec.sourceNamespace", func() {
		It("rejects an empty source namespace", func() {
			ns := newSync("empty-source")
			ns.Spec.SourceNamespace = ""

			err := k8sClient.Create(ctx, ns)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.sourceNamespace"))
		})

		It("rejects a source namespace that is not a DNS-1123 label", func() {
			ns := newSync("bad-source")
			ns.Spec.SourceNamespace = "Not_A_Namespace"

			err := k8sClient.Create(ctx, ns)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.sourceNamespace"))
		})

		It("accepts a valid source namespace", func() {
			createOK(newSync("good-source"))
		})
	})

	Context("at least one resource to sync", func() {
		It("rejects a spec with neither secretName nor configMapName", func() {
			ns := newSync("no-resources")
			ns.Spec.ConfigMapName = nil

			err := k8sClient.Create(ctx, ns)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one secret or configmap must be specified"))
		})

		It("rejects a spec with only empty resource lists", func() {
			ns := newSync("empty-resources")
			ns.Spec.ConfigMapName = []string{}
			ns.Spec.SecretName = []string{}

			err := k8sClient.Create(ctx, ns)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one secret or configmap must be specified"))
		})

		It("accepts a spec with only secretName", func() {
			ns := newSync("only-secret")
			ns.Spec.ConfigMapName = nil
			ns.Spec.SecretName = []string{"crd-validation-secret"}

			createOK(ns)
		})

		It("accepts a spec with both secretName and configMapName", func() {
			ns := newSync("both-resources")
			ns.Spec.SecretName = []string{"crd-validation-secret"}

			createOK(ns)
		})

		It("rejects an update that removes the last synced resource", func() {
			ns := newSync("update-guard")
			createOK(ns)

			// The controller adds a finalizer and writes status right after create,
			// so the local copy goes stale. Re-read inside the retry: updating from
			// the stale copy races into a 409 conflict instead of the CEL rejection
			// this spec is asserting.
			Eventually(func(g Gomega) {
				latest := &syncv1.NamespaceSync{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ns), latest)).To(Succeed())
				latest.Spec.ConfigMapName = nil

				err := k8sClient.Update(ctx, latest)
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("at least one secret or configmap must be specified"))
			}, time.Second*10, time.Millisecond*250).Should(Succeed())
		})
	})
})
