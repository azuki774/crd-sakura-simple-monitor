package controller

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitoringv1alpha1 "github.com/azuki774/crd-sakura-simple-monitor/api/v1alpha1"
)

var _ = Describe("SakuraSimpleMonitor CRD installation", func() {
	It("makes SakuraSimpleMonitor discoverable after CRD installation", func() {
		// envtest は suite_test.go の CRDDirectoryPaths で config/crd/bases を API server に登録する。
		// ここでは make install 後の kubectl api-resources に相当する状態を discovery API で確認する。
		discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		resources, err := discoveryClient.ServerResourcesForGroupVersion("monitoring.k8s.azuki.blue/v1alpha1")
		Expect(err).NotTo(HaveOccurred())

		var sakuraSimpleMonitorResource *metav1.APIResource
		for i := range resources.APIResources {
			resource := resources.APIResources[i]
			if resource.Name == "sakurasimplemonitors" {
				sakuraSimpleMonitorResource = &resource
				break
			}
		}

		Expect(sakuraSimpleMonitorResource).NotTo(BeNil())
		Expect(sakuraSimpleMonitorResource.Kind).To(Equal("SakuraSimpleMonitor"))
		Expect(sakuraSimpleMonitorResource.Namespaced).To(BeTrue())
		Expect(sakuraSimpleMonitorResource.Verbs).To(ContainElements(
			"create",
			"delete",
			"get",
			"list",
			"patch",
			"update",
			"watch",
		))
	})

	It("enforces the SakuraSimpleMonitor OpenAPI schema", func() {
		// Go の型ではなく unstructured を使い、CRD の OpenAPI schema が API server 側で効くことを見る。
		// valid な CR は作成でき、schema に反する CR は作成時に拒否される。
		tests := []struct {
			name          string
			monitor       *unstructured.Unstructured
			wantError     bool
			wantErrorText string
		}{
			{
				name:    "accepts a valid https monitor",
				monitor: newSakuraSimpleMonitorObject("crd-installation-valid", "https"),
			},
			{
				name:          "rejects an unsupported health check protocol",
				monitor:       newSakuraSimpleMonitorObject("crd-installation-invalid-protocol", "ftp"),
				wantError:     true,
				wantErrorText: "ftp",
			},
		}

		for _, tt := range tests {
			tt := tt
			By(tt.name)

			err := k8sClient.Create(ctx, tt.monitor)
			if tt.wantError {
				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsInvalid(err) || apierrors.IsBadRequest(err)).To(BeTrue(), "unexpected error: %v", err)
				Expect(strings.Contains(err.Error(), tt.wantErrorText)).To(BeTrue(), "expected validation error to mention %q: %v", tt.wantErrorText, err)
				continue
			}

			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				current := &monitoringv1alpha1.SakuraSimpleMonitor{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(tt.monitor), current)
				if apierrors.IsNotFound(err) {
					return
				}
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Delete(ctx, current)).To(Succeed())
			})

			fetched := &monitoringv1alpha1.SakuraSimpleMonitor{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(tt.monitor), fetched)).To(Succeed())
			Expect(fetched.Spec.Target).To(Equal("example.com"))
		}
	})
})

func newSakuraSimpleMonitorObject(name, protocol string) *unstructured.Unstructured {
	monitor := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "monitoring.k8s.azuki.blue/v1alpha1",
		"kind":       "SakuraSimpleMonitor",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": "default",
		},
		"spec": map[string]interface{}{
			"target": "example.com",
			"healthCheck": map[string]interface{}{
				"protocol":       protocol,
				"port":           int64(443),
				"path":           "/healthz",
				"expectedStatus": int64(200),
				"timeoutSeconds": int64(10),
			},
			"interval":      int64(1),
			"retryInterval": int64(20),
			"notifications": map[string]interface{}{
				"webhookURL":     "https://example.com/webhook",
				"message":        "example.com health check failed",
				"repeatInterval": int64(7200),
			},
			"description": "CRD installation test resource",
		},
	}}
	monitor.SetGroupVersionKind(monitoringv1alpha1.GroupVersion.WithKind("SakuraSimpleMonitor"))
	return monitor
}
