/*
Copyright 2026 azuki774.

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	monitoringv1alpha1 "github.com/azuki774/crd-sakura-simple-monitor/api/v1alpha1"
)

var _ = Describe("SakuraSimpleMonitor Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		sakurasimplemonitor := &monitoringv1alpha1.SakuraSimpleMonitor{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind SakuraSimpleMonitor")
			err := k8sClient.Get(ctx, typeNamespacedName, sakurasimplemonitor)
			if err != nil && errors.IsNotFound(err) {
				resource := &monitoringv1alpha1.SakuraSimpleMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: monitoringv1alpha1.SakuraSimpleMonitorSpec{
						Target: "example.com",
						HealthCheck: monitoringv1alpha1.HealthCheckSpec{
							Protocol:       monitoringv1alpha1.HealthCheckProtocolHTTPS,
							Port:           443,
							Path:           "/healthz",
							ExpectedStatus: 200,
							TimeoutSeconds: 10,
						},
						Interval:      1,
						RetryInterval: 20,
						Notifications: monitoringv1alpha1.NotificationsSpec{
							WebhookURL:     "https://example.com/webhook",
							Message:        "example.com health check failed",
							RepeatInterval: 7200,
						},
						Description: "test monitor",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &monitoringv1alpha1.SakuraSimpleMonitor{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if errors.IsNotFound(err) {
				return
			}
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance SakuraSimpleMonitor")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		DescribeTable("should reconcile without side effects",
			func(name string, expectResource bool) {
				controllerReconciler := &SakuraSimpleMonitorReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      name,
						Namespace: "default",
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				Expect(result.RequeueAfter).To(BeZero())

				if !expectResource {
					return
				}

				resource := &monitoringv1alpha1.SakuraSimpleMonitor{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
				Expect(resource.Status.MonitorID).To(BeEmpty())
				Expect(resource.Status.Health).To(BeEmpty())
				Expect(resource.Status.ObservedGeneration).To(BeZero())
				Expect(resource.Status.Conditions).To(BeEmpty())
				Expect(resource.Status.LastSyncedAt).To(BeNil())
			},
			Entry("existing resource", resourceName, true),
			Entry("missing resource", "missing-resource", false),
		)
	})
})
