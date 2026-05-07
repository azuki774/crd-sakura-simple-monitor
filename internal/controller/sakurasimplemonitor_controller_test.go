package controller

import (
	"context"
	"errors"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
					return reconcile.Request{NamespacedName: clientKey(resource)}, &fakeSimpleMonitorClient{
						createID: "123456789012",
					}
				},
				wantErr: expectNoReconcileError,
				verify: func(result reconcile.Result, fakeSakura *fakeSimpleMonitorClient) {
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(Equal(syncVerificationInterval))

					Expect(fakeSakura.creates).To(HaveLen(1))
					Expect(fakeSakura.creates[0].Tags).To(BeEmpty())
					Expect(fakeSakura.creates[0].Target).To(Equal("example.com"))
					Expect(fakeSakura.creates[0].Protocol).To(Equal(monitoringv1alpha1.HealthCheckProtocolHTTPS))
					Expect(fakeSakura.creates[0].HTTP2).To(BeTrue())

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
			// status.monitorID がある未処理 generation の CR は、事前 Read せず、その ID のまま更新する。
			Entry("updates the SakuraCloud simple monitor when status already has a monitor ID", reconcileCase{
				note: "保存済み monitorID を使って SakuraCloud シンプル監視を更新し、新規作成は行わない",
				setup: func() (reconcile.Request, *fakeSimpleMonitorClient) {
					resource := createTestMonitor(ctx, "update-resource")
					setMonitorID(ctx, resource, "123456789012")
					return reconcile.Request{NamespacedName: clientKey(resource)}, &fakeSimpleMonitorClient{}
				},
				wantErr: expectNoReconcileError,
				verify: func(_ reconcile.Result, fakeSakura *fakeSimpleMonitorClient) {
					Expect(fakeSakura.updates).To(HaveLen(1))
					Expect(fakeSakura.updates[0].id).To(Equal("123456789012"))
					Expect(fakeSakura.updates[0].desired.Tags).To(BeEmpty())
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
						createID:  "234567890123",
						updateErr: simplemonitor.ErrSimpleMonitorNotFound,
					}
				},
				wantErr: expectNoReconcileError,
				verify: func(_ reconcile.Result, fakeSakura *fakeSimpleMonitorClient) {
					Expect(fakeSakura.creates).To(HaveLen(1))
					Expect(fakeSakura.updates).To(HaveLen(1))
					Expect(fakeSakura.updates[0].id).To(Equal("123456789012"))

					fetched := getTestMonitor(ctx, "recreate-resource")
					Expect(fetched.Status.MonitorID).To(Equal("234567890123"))
				},
			}),
			// Sakura API エラーは Kubernetes 側 status に失敗 Condition として残し、controller-runtime の自動再試行は起こさない。
			Entry("records a failed sync condition when the SakuraCloud API returns an error", reconcileCase{
				note: "SakuraCloud API が失敗した場合は失敗 Condition を status に記録し、Reconcile error は返さない",
				setup: func() (reconcile.Request, *fakeSimpleMonitorClient) {
					resource := createTestMonitor(ctx, "error-resource")
					return reconcile.Request{NamespacedName: clientKey(resource)}, &fakeSimpleMonitorClient{
						createErr: errors.New("sakura api unavailable"),
					}
				},
				wantErr: expectNoReconcileError,
				verify: func(result reconcile.Result, _ *fakeSimpleMonitorClient) {
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())
					fetched := getTestMonitor(ctx, "error-resource")
					Expect(fetched.Status.MonitorID).To(BeEmpty())
					Expect(fetched.Status.ObservedGeneration).To(Equal(fetched.Generation))
					condition := findCondition(fetched.Status.Conditions, conditionTypeSynced)
					Expect(condition).NotTo(BeNil())
					Expect(condition.Status).To(Equal(metav1.ConditionFalse))
					Expect(condition.Reason).To(Equal(syncReasonFailed))
					Expect(condition.Message).To(ContainSubstring("sakura api unavailable"))
				},
			}),
			// 同期済み generation は status-only update の再リコンサイルでも SakuraCloud API に触らない。
			Entry("skips SakuraCloud API calls when the current generation is already synchronized", reconcileCase{
				note: "現在の generation が同期済みで 24 時間以内に確認済みなら SakuraCloud API を呼ばず終了する",
				setup: func() (reconcile.Request, *fakeSimpleMonitorClient) {
					resource := createTestMonitor(ctx, "synced-resource")
					setSyncStatus(ctx, resource, "123456789012", metav1.ConditionTrue, syncReasonSucceeded, "synced")
					return reconcile.Request{NamespacedName: clientKey(resource)}, &fakeSimpleMonitorClient{}
				},
				wantErr: expectNoReconcileError,
				verify: func(result reconcile.Result, fakeSakura *fakeSimpleMonitorClient) {
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(Equal(syncVerificationInterval))
					Expect(fakeSakura.called()).To(BeFalse())
				},
			}),
			// 同期済み generation でも 24 時間経過後は SakuraCloud API の GET で設定値の同期状態を確認する。
			Entry("checks SakuraCloud synchronization when the current generation is due for verification", reconcileCase{
				note: "最後の同期確認から 24 時間経過している場合は SakuraCloud シンプル監視を GET して desired state と一致することを確認する",
				setup: func() (reconcile.Request, *fakeSimpleMonitorClient) {
					resource := createTestMonitor(ctx, "verification-due-resource")
					setSyncStatusAt(ctx, resource, "123456789012", metav1.ConditionTrue, syncReasonSucceeded, "synced", testNow().Add(-syncVerificationInterval))
					return reconcile.Request{NamespacedName: clientKey(resource)}, &fakeSimpleMonitorClient{}
				},
				wantErr: expectNoReconcileError,
				verify: func(result reconcile.Result, fakeSakura *fakeSimpleMonitorClient) {
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(Equal(syncVerificationInterval))
					Expect(fakeSakura.checks).To(HaveLen(1))
					Expect(fakeSakura.checks[0].id).To(Equal("123456789012"))
					Expect(fakeSakura.creates).To(BeEmpty())
					Expect(fakeSakura.updates).To(BeEmpty())

					fetched := getTestMonitor(ctx, "verification-due-resource")
					Expect(fetched.Status.LastSyncedAt).NotTo(BeNil())
					Expect(fetched.Status.LastSyncedAt.Time).To(BeTemporally("==", testNow()))
					condition := findCondition(fetched.Status.Conditions, conditionTypeSynced)
					Expect(condition).NotTo(BeNil())
					Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				},
			}),
			// 24 時間確認で Sakura 側との差分を検出した場合は、エラー状態で止める。
			Entry("records a failed sync condition when verification finds drift", reconcileCase{
				note: "SakuraCloud 側の設定が desired state と一致しない場合は失敗 Condition を status に記録し、Reconcile error は返さない",
				setup: func() (reconcile.Request, *fakeSimpleMonitorClient) {
					resource := createTestMonitor(ctx, "verification-drift-resource")
					setSyncStatusAt(ctx, resource, "123456789012", metav1.ConditionTrue, syncReasonSucceeded, "synced", testNow().Add(-syncVerificationInterval))
					return reconcile.Request{NamespacedName: clientKey(resource)}, &fakeSimpleMonitorClient{
						checkErr: errors.New("SakuraCloud simple monitor is out of sync: [healthCheck.path]"),
					}
				},
				wantErr: expectNoReconcileError,
				verify: func(result reconcile.Result, fakeSakura *fakeSimpleMonitorClient) {
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())
					Expect(fakeSakura.checks).To(HaveLen(1))

					fetched := getTestMonitor(ctx, "verification-drift-resource")
					Expect(fetched.Status.ObservedGeneration).To(Equal(fetched.Generation))
					condition := findCondition(fetched.Status.Conditions, conditionTypeSynced)
					Expect(condition).NotTo(BeNil())
					Expect(condition.Status).To(Equal(metav1.ConditionFalse))
					Expect(condition.Message).To(ContainSubstring("out of sync"))
				},
			}),
			// 失敗済み generation も自動再試行せず、次の spec 変更までエラー状態で止める。
			Entry("skips SakuraCloud API calls when the current generation already failed", reconcileCase{
				note: "現在の generation が失敗済みなら SakuraCloud API を呼ばず終了する",
				setup: func() (reconcile.Request, *fakeSimpleMonitorClient) {
					resource := createTestMonitor(ctx, "failed-resource")
					setSyncStatus(ctx, resource, "", metav1.ConditionFalse, syncReasonFailed, "sakura api unavailable")
					return reconcile.Request{NamespacedName: clientKey(resource)}, &fakeSimpleMonitorClient{}
				},
				wantErr: expectNoReconcileError,
				verify: func(result reconcile.Result, fakeSakura *fakeSimpleMonitorClient) {
					Expect(result.Requeue).To(BeFalse())
					Expect(fakeSakura.called()).To(BeFalse())
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
	checkErr  error
	updateErr error
	creates   []simplemonitor.SimpleMonitorDesired
	checks    []struct {
		id      string
		desired simplemonitor.SimpleMonitorDesired
	}
	updates []struct {
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

func (f *fakeSimpleMonitorClient) CheckSynced(_ context.Context, id string, desired simplemonitor.SimpleMonitorDesired) error {
	f.checks = append(f.checks, struct {
		id      string
		desired simplemonitor.SimpleMonitorDesired
	}{id: id, desired: desired})
	return f.checkErr
}

func (f *fakeSimpleMonitorClient) Update(_ context.Context, id string, desired simplemonitor.SimpleMonitorDesired) error {
	f.updates = append(f.updates, struct {
		id      string
		desired simplemonitor.SimpleMonitorDesired
	}{id: id, desired: desired})
	return f.updateErr
}

func (f *fakeSimpleMonitorClient) called() bool {
	return len(f.creates) > 0 || len(f.checks) > 0 || len(f.updates) > 0
}

func newTestReconciler(simpleMonitorClient simplemonitor.SimpleMonitorClient) *SakuraSimpleMonitorReconciler {
	return &SakuraSimpleMonitorReconciler{
		Client:              k8sClient,
		Scheme:              k8sClient.Scheme(),
		SakuraSimpleMonitor: simpleMonitorClient,
		Now: func() time.Time {
			return testNow()
		},
	}
}

func testNow() time.Time {
	return time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
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
				HTTP2:          true,
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

func setSyncStatus(
	ctx context.Context,
	resource *monitoringv1alpha1.SakuraSimpleMonitor,
	monitorID string,
	conditionStatus metav1.ConditionStatus,
	reason string,
	message string,
) {
	setSyncStatusAt(ctx, resource, monitorID, conditionStatus, reason, message, testNow())
}

func setSyncStatusAt(
	ctx context.Context,
	resource *monitoringv1alpha1.SakuraSimpleMonitor,
	monitorID string,
	conditionStatus metav1.ConditionStatus,
	reason string,
	message string,
	lastSyncedAt time.Time,
) {
	current := &monitoringv1alpha1.SakuraSimpleMonitor{}
	Expect(k8sClient.Get(ctx, clientKey(resource), current)).To(Succeed())
	current.Status.MonitorID = monitorID
	current.Status.ObservedGeneration = current.Generation
	if conditionStatus == metav1.ConditionTrue {
		syncedAt := metav1.NewTime(lastSyncedAt)
		current.Status.LastSyncedAt = &syncedAt
	}
	meta.SetStatusCondition(&current.Status.Conditions, metav1.Condition{
		Type:               conditionTypeSynced,
		Status:             conditionStatus,
		ObservedGeneration: current.Generation,
		Reason:             reason,
		Message:            message,
	})
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

func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	index := slices.IndexFunc(conditions, func(condition metav1.Condition) bool {
		return condition.Type == conditionType
	})
	if index == -1 {
		return nil
	}
	return &conditions[index]
}
