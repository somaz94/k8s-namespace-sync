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
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	// +kubebuilder:scaffold:imports
)

var (
	cfg        *rest.Config
	k8sClient  client.Client
	testEnv    *envtest.Environment
	cancel     context.CancelFunc
	k8sManager ctrl.Manager
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	logf.SetLogger(zap.New(
		zap.WriteTo(GinkgoWriter),
		zap.UseDevMode(true),
		zap.Level(zapcore.DebugLevel),
	))

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func(ctx context.Context) {
	opts := zap.Options{
		Development: true,
		Level:       zapcore.DebugLevel,
	}
	logf.SetLogger(zap.New(
		zap.WriteTo(GinkgoWriter),
		zap.UseDevMode(true),
		zap.Level(opts.Level),
	))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,

		// Used only by a plain `go test`; `make test` sets KUBEBUILDER_ASSETS, which wins.
		// The version must match ENVTEST_K8S_VERSION in the Makefile.
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s",
			fmt.Sprintf("1.31.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = syncv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: server.Options{
			BindAddress: "0",
		},
		LeaderElection: false,
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&NamespaceSyncReconciler{
		Client:    k8sManager.GetClient(),
		Scheme:    k8sManager.GetScheme(),
		APIReader: k8sManager.GetAPIReader(),
		// SA1019: matches the production wiring in cmd/main.go; see the note there.
		//nolint:staticcheck
		Recorder: k8sManager.GetEventRecorderFor("namespacesync-controller"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()
})

var _ = AfterSuite(func(ctx SpecContext) {
	By("tearing down the test environment")
	cancel()

	cleanupCtx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	done := make(chan error)
	go func() {
		done <- testEnv.Stop()
		close(done)
	}()

	select {
	case err := <-done:
		if err != nil {
			By(fmt.Sprintf("got an error during cleanup: %v", err))
		}
	case <-cleanupCtx.Done():
		By("cleanup timed out, forcing exit")
	}
}, NodeTimeout(time.Minute))
