package controller

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	monitoringv1alpha1 "github.com/azuki774/crd-sakura-simple-monitor/api/v1alpha1"
	"github.com/azuki774/crd-sakura-simple-monitor/internal/simplemonitor"
)

var _ = Describe("SakuraSimpleMonitor Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()

		type reconcileCase struct {
			note    string
			setup   func() (reconcile.Request, *fakeSimpleMonitorClient)
			wantErr func(error)
			verify  func(reconcile.Result, *fakeSimpleMonitorClient)
		}

		DescribeTable("syncs SakuraCloud simple monitors",
			func(tt reconcileCase) {
				By(tt.note)
				req, fakeSakura := tt.setup()
				reconciler := newTestReconciler(fakeSakura)

				result, err := reconciler.Reconcile(ctx, req)
				tt.wantErr(err)
				tt.verify(result, fakeSakura)
			},
			// status.monitorID が空の CR は、controller が所有する Sakura 側リソースとして新規作成する。
			Entry("creates a SakuraCloud simple monitor and stores the synchronized status", reconcileCase{
				note: "status.monitorID が空の CR から SakuraCloud シンプル監視を作成し、ID と同期状態を status に保存する",
				setup: func() (reconcile.Request, *fakeSimpleMonitorClient) {
					resource := createTestMonitor(ctx, "create-resource")
					return reconcile.Request{NamespacedName: clientKey(resource)}, &fakeSimpleMonitorClient{createID: "123456789012"}
				},
				wantErr: expectNoReconcileError,
				verify: func(result reconcile.Result, fakeSakura *fakeSimpleMonitorClient) {
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())

					Expect(fakeSakura.creates).To(HaveLen(1))
					Expect(fakeSakura.creates[0].Tags).To(ContainElements(
						"managed-by=crd-sakura-simple-monitor",
						"k8s-kind=SakuraSimpleMonitor",
						"k8s-namespace=default",
						"k8s-name=create-resource",
						"k8s-resource=default-create-resource",
						findTagWithPrefix(fakeSakura.creates[0].Tags, "k8s-uid="),
					))
					Expect(fakeSakura.creates[0].Target).To(Equal("example.com"))
					Expect(fakeSakura.creates[0].Protocol).To(Equal(monitoringv1alpha1.HealthCheckProtocolHTTPS))

					fetched := getTestMonitor(ctx, "create-resource")
					Expect(fetched.Status.MonitorID).To(Equal("123456789012"))
					Expect(fetched.Status.ObservedGeneration).To(Equal(fetched.Generation))
					Expect(fetched.Status.LastSyncedAt).NotTo(BeNil())
					condition := findCondition(fetched.Status.Conditions, conditionTypeSynced)
					Expect(condition).NotTo(BeNil())
					Expect(condition.Status).To(Equal(metav1.ConditionTrue))
					Expect(condition.Reason).To(Equal(syncReasonSucceeded))
				},
			}),
			// status.monitorID がある CR は、既存 Sakura 側リソースを ID で読み取り、その ID のまま更新する。
			Entry("updates the SakuraCloud simple monitor when status already has a monitor ID", reconcileCase{
				note: "保存済み monitorID を使って SakuraCloud シンプル監視を更新し、新規作成は行わない",
				setup: func() (reconcile.Request, *fakeSimpleMonitorClient) {
					resource := createTestMonitor(ctx, "update-resource")
					setMonitorID(ctx, resource, "123456789012")
					return reconcile.Request{NamespacedName: clientKey(resource)}, &fakeSimpleMonitorClient{}
				},
				wantErr: expectNoReconcileError,
				verify: func(_ reconcile.Result, fakeSakura *fakeSimpleMonitorClient) {
					Expect(fakeSakura.reads).To(Equal([]string{"123456789012"}))
					Expect(fakeSakura.updates).To(HaveLen(1))
					Expect(fakeSakura.updates[0].id).To(Equal("123456789012"))
					Expect(fakeSakura.updates[0].desired.Tags).To(ContainElement("k8s-name=update-resource"))
					Expect(fakeSakura.updates[0].desired.Tags).To(ContainElement("k8s-resource=default-update-resource"))
					Expect(fakeSakura.creates).To(BeEmpty())
				},
			}),
			// Sakura 側リソースが手動削除された場合は、CR の desired state から再作成して monitorID を差し替える。
			Entry("recreates the SakuraCloud simple monitor when the stored monitor ID is not found", reconcileCase{
				note: "status.monitorID の SakuraCloud リソースが見つからない場合は再作成し、新しい ID を status に保存する",
				setup: func() (reconcile.Request, *fakeSimpleMonitorClient) {
					resource := createTestMonitor(ctx, "recreate-resource")
					setMonitorID(ctx, resource, "123456789012")
					return reconcile.Request{NamespacedName: clientKey(resource)}, &fakeSimpleMonitorClient{
						createID: "234567890123",
						readErr:  simplemonitor.ErrSimpleMonitorNotFound,
					}
				},
				wantErr: expectNoReconcileError,
				verify: func(_ reconcile.Result, fakeSakura *fakeSimpleMonitorClient) {
					Expect(fakeSakura.reads).To(Equal([]string{"123456789012"}))
					Expect(fakeSakura.creates).To(HaveLen(1))
					Expect(fakeSakura.updates).To(BeEmpty())

					fetched := getTestMonitor(ctx, "recreate-resource")
					Expect(fetched.Status.MonitorID).To(Equal("234567890123"))
				},
			}),
			// Sakura API エラーは握りつぶさず、Kubernetes 側 status に失敗 Condition として残す。
			Entry("records a failed sync condition when the SakuraCloud API returns an error", reconcileCase{
				note: "SakuraCloud API が失敗した場合は Reconcile error を返し、失敗 Condition を status に記録する",
				setup: func() (reconcile.Request, *fakeSimpleMonitorClient) {
					resource := createTestMonitor(ctx, "error-resource")
					return reconcile.Request{NamespacedName: clientKey(resource)}, &fakeSimpleMonitorClient{
						createErr: errors.New("sakura api unavailable"),
					}
				},
				wantErr: func(err error) {
					Expect(err).To(MatchError("sakura api unavailable"))
				},
				verify: func(_ reconcile.Result, _ *fakeSimpleMonitorClient) {
					fetched := getTestMonitor(ctx, "error-resource")
					Expect(fetched.Status.MonitorID).To(BeEmpty())
					condition := findCondition(fetched.Status.Conditions, conditionTypeSynced)
					Expect(condition).NotTo(BeNil())
					Expect(condition.Status).To(Equal(metav1.ConditionFalse))
					Expect(condition.Reason).To(Equal(syncReasonFailed))
					Expect(condition.Message).To(ContainSubstring("sakura api unavailable"))
				},
			}),
			// Kubernetes API 上に CR が存在しない場合は、削除済みとして外部 API に触らず正常終了する。
			Entry("ignores missing Kubernetes resources", reconcileCase{
				note: "対象 CR が存在しない場合は SakuraCloud API を呼ばず、再キューもしない",
				setup: func() (reconcile.Request, *fakeSimpleMonitorClient) {
					return reconcile.Request{
						NamespacedName: types.NamespacedName{Name: "missing-resource", Namespace: "default"},
					}, &fakeSimpleMonitorClient{}
				},
				wantErr: expectNoReconcileError,
				verify: func(result reconcile.Result, fakeSakura *fakeSimpleMonitorClient) {
					Expect(result.Requeue).To(BeFalse())
					Expect(fakeSakura.called()).To(BeFalse())
				},
			}),
			// DeletionTimestamp が付いた CR は Create/Update only の範囲外なので、外部 API を呼ばない。
			Entry("does not call SakuraCloud API for deleting resources", reconcileCase{
				note: "削除中 CR は初回実装の同期対象外として扱い、SakuraCloud API を呼ばない",
				setup: func() (reconcile.Request, *fakeSimpleMonitorClient) {
					resource := createTestMonitor(ctx, "deleting-resource")
					resource.Finalizers = []string{"test.monitoring.k8s.azuki.blue/finalizer"}
					Expect(k8sClient.Update(ctx, resource)).To(Succeed())
					Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
					DeferCleanup(func() {
						current := &monitoringv1alpha1.SakuraSimpleMonitor{}
						if err := k8sClient.Get(ctx, clientKey(resource), current); apierrors.IsNotFound(err) {
							return
						}
						current.Finalizers = nil
						Expect(k8sClient.Update(ctx, current)).To(Succeed())
					})
					return reconcile.Request{NamespacedName: clientKey(resource)}, &fakeSimpleMonitorClient{}
				},
				wantErr: expectNoReconcileError,
				verify: func(_ reconcile.Result, fakeSakura *fakeSimpleMonitorClient) {
					Expect(fakeSakura.called()).To(BeFalse())
				},
			}),
		)
	})
})

type fakeSimpleMonitorClient struct {
	createID  string
	createErr error
	readErr   error
	updateErr error
	creates   []simplemonitor.SimpleMonitorDesired
	reads     []string
	updates   []struct {
		id      string
		desired simplemonitor.SimpleMonitorDesired
	}
}

func (f *fakeSimpleMonitorClient) Create(_ context.Context, desired simplemonitor.SimpleMonitorDesired) (string, error) {
	f.creates = append(f.creates, desired)
	if f.createErr != nil {
		return "", f.createErr
	}
	if f.createID == "" {
		return "123456789012", nil
	}
	return f.createID, nil
}

func (f *fakeSimpleMonitorClient) Read(_ context.Context, id string) error {
	f.reads = append(f.reads, id)
	return f.readErr
}

func (f *fakeSimpleMonitorClient) Update(_ context.Context, id string, desired simplemonitor.SimpleMonitorDesired) error {
	f.updates = append(f.updates, struct {
		id      string
		desired simplemonitor.SimpleMonitorDesired
	}{id: id, desired: desired})
	return f.updateErr
}

func (f *fakeSimpleMonitorClient) called() bool {
	return len(f.creates) > 0 || len(f.reads) > 0 || len(f.updates) > 0
}

func newTestReconciler(simpleMonitorClient simplemonitor.SimpleMonitorClient) *SakuraSimpleMonitorReconciler {
	return &SakuraSimpleMonitorReconciler{
		Client:              k8sClient,
		Scheme:              k8sClient.Scheme(),
		SakuraSimpleMonitor: simpleMonitorClient,
		Now: func() time.Time {
			return time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		},
	}
}

func createTestMonitor(ctx context.Context, name string) *monitoringv1alpha1.SakuraSimpleMonitor {
	resource := &monitoringv1alpha1.SakuraSimpleMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
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
	DeferCleanup(func() {
		current := &monitoringv1alpha1.SakuraSimpleMonitor{}
		err := k8sClient.Get(ctx, clientKey(resource), current)
		if apierrors.IsNotFound(err) {
			return
		}
		Expect(err).NotTo(HaveOccurred())
		current.Finalizers = nil
		Expect(k8sClient.Update(ctx, current)).To(Succeed())
		Expect(k8sClient.Delete(ctx, current)).To(Succeed())
	})

	fetched := &monitoringv1alpha1.SakuraSimpleMonitor{}
	Expect(k8sClient.Get(ctx, clientKey(resource), fetched)).To(Succeed())
	return fetched
}

func setMonitorID(ctx context.Context, resource *monitoringv1alpha1.SakuraSimpleMonitor, monitorID string) {
	current := &monitoringv1alpha1.SakuraSimpleMonitor{}
	Expect(k8sClient.Get(ctx, clientKey(resource), current)).To(Succeed())
	current.Status.MonitorID = monitorID
	Expect(k8sClient.Status().Update(ctx, current)).To(Succeed())
}

func getTestMonitor(ctx context.Context, name string) *monitoringv1alpha1.SakuraSimpleMonitor {
	current := &monitoringv1alpha1.SakuraSimpleMonitor{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, current)).To(Succeed())
	return current
}

func expectNoReconcileError(err error) {
	Expect(err).NotTo(HaveOccurred())
}

func clientKey(resource *monitoringv1alpha1.SakuraSimpleMonitor) types.NamespacedName {
	return types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}
}

func findTagWithPrefix(tags []string, prefix string) string {
	index := slices.IndexFunc(tags, func(tag string) bool {
		return strings.HasPrefix(tag, prefix)
	})
	if index == -1 {
		return ""
	}
	return tags[index]
}

func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	index := slices.IndexFunc(conditions, func(condition metav1.Condition) bool {
		return condition.Type == conditionType
	})
	if index == -1 {
		return nil
	}
	return &conditions[index]
}
