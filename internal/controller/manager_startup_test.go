package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var _ = Describe("Manager startup", func() {
	It("starts the no-op controller without external configuration", func() {
		mgr, err := ctrl.NewManager(cfg, manager.Options{
			Scheme:                 k8sClient.Scheme(),
			HealthProbeBindAddress: "0",
			Metrics: metricsserver.Options{
				BindAddress: "0",
			},
			Cache: cache.Options{
				DefaultNamespaces: map[string]cache.Config{
					"default": {},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		err = (&SakuraSimpleMonitorReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr)
		Expect(err).NotTo(HaveOccurred())

		startCtx, stop := context.WithCancel(ctx)
		defer stop()

		errCh := make(chan error, 1)
		go func() {
			errCh <- mgr.Start(startCtx)
		}()

		Eventually(func() bool {
			select {
			case err := <-errCh:
				Expect(err).NotTo(HaveOccurred())
				return false
			default:
			}

			cacheSyncCtx, cancel := context.WithTimeout(startCtx, 100*time.Millisecond)
			defer cancel()

			return mgr.GetCache().WaitForCacheSync(cacheSyncCtx)
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())
		Consistently(errCh, 300*time.Millisecond, 50*time.Millisecond).ShouldNot(Receive())

		stop()
		Eventually(errCh, 5*time.Second, 100*time.Millisecond).Should(Receive(Succeed()))
	})
})
