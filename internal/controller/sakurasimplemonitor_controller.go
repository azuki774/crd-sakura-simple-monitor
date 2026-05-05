package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/azuki774/crd-sakura-simple-monitor/api/v1alpha1"
	"github.com/azuki774/crd-sakura-simple-monitor/internal/simplemonitor"
)

const (
	conditionTypeSynced = "Synced"

	syncReasonSucceeded = "SyncSucceeded"
	syncReasonFailed    = "SyncFailed"

	managedByTag = "managed-by-crd-sakura-simple-monitor"
	kindTag      = "k8s-kind-sakurasimplemonitor"
)

// SakuraSimpleMonitorReconciler reconciles a SakuraSimpleMonitor object
type SakuraSimpleMonitorReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	SakuraSimpleMonitor simplemonitor.SimpleMonitorClient
	Now                 func() time.Time
}

// +kubebuilder:rbac:groups=monitoring.k8s.azuki.blue,resources=sakurasimplemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.k8s.azuki.blue,resources=sakurasimplemonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.k8s.azuki.blue,resources=sakurasimplemonitors/finalizers,verbs=update

// Reconcile creates or updates the SakuraCloud simple monitor that corresponds to the Kubernetes resource.
func (r *SakuraSimpleMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	monitor := &monitoringv1alpha1.SakuraSimpleMonitor{}
	if err := r.Get(ctx, req.NamespacedName, monitor); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !monitor.DeletionTimestamp.IsZero() {
		logger.Info("resource is deleting", "name", monitor.Name, "namespace", monitor.Namespace)
		return ctrl.Result{}, nil
	}

	if r.SakuraSimpleMonitor == nil {
		err := errors.New("sakura simple monitor client is not configured")
		return ctrl.Result{}, r.setSyncFailed(ctx, monitor, err)
	}

	desired := desiredSimpleMonitor(monitor)
	var monitorID string
	var err error

	if monitor.Status.MonitorID == "" {
		monitorID, err = r.SakuraSimpleMonitor.Create(ctx, desired)
	} else {
		monitorID = monitor.Status.MonitorID
		err = r.SakuraSimpleMonitor.Read(ctx, monitorID)
		if errors.Is(err, simplemonitor.ErrSimpleMonitorNotFound) {
			monitorID, err = r.SakuraSimpleMonitor.Create(ctx, desired)
		} else if err == nil {
			err = r.SakuraSimpleMonitor.Update(ctx, monitorID, desired)
		}
	}

	if err != nil {
		logger.Error(err, "failed to synchronize SakuraCloud simple monitor", "monitorID", monitor.Status.MonitorID)
		return ctrl.Result{}, r.setSyncFailed(ctx, monitor, err)
	}

	if monitorID == "" {
		err = errors.New("sakura simple monitor API returned an empty monitor ID")
		return ctrl.Result{}, r.setSyncFailed(ctx, monitor, err)
	}

	logger.Info(
		"synchronized SakuraCloud simple monitor",
		"name", monitor.Name,
		"namespace", monitor.Namespace,
		"target", monitor.Spec.Target,
		"protocol", monitor.Spec.HealthCheck.Protocol,
		"monitorID", monitorID,
	)

	return ctrl.Result{}, r.setSyncSucceeded(ctx, monitor, monitorID)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SakuraSimpleMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.SakuraSimpleMonitor{}).
		Named("sakurasimplemonitor").
		Complete(r)
}

func desiredSimpleMonitor(monitor *monitoringv1alpha1.SakuraSimpleMonitor) simplemonitor.SimpleMonitorDesired {
	return simplemonitor.SimpleMonitorDesired{
		Target:         monitor.Spec.Target,
		Description:    monitor.Spec.Description,
		Tags:           resourceTags(monitor),
		Protocol:       monitor.Spec.HealthCheck.Protocol,
		Port:           monitor.Spec.HealthCheck.Port,
		Path:           monitor.Spec.HealthCheck.Path,
		ExpectedStatus: monitor.Spec.HealthCheck.ExpectedStatus,
		TimeoutSeconds: monitor.Spec.HealthCheck.TimeoutSeconds,
		Interval:       monitor.Spec.Interval,
		RetryInterval:  monitor.Spec.RetryInterval,
		WebhookURL:     monitor.Spec.Notifications.WebhookURL,
		RepeatInterval: monitor.Spec.Notifications.RepeatInterval,
	}
}

func resourceTags(monitor *monitoringv1alpha1.SakuraSimpleMonitor) []string {
	return []string{
		managedByTag,
		kindTag,
		fmt.Sprintf("k8s-namespace-%s", monitor.Namespace),
		fmt.Sprintf("k8s-name-%s", monitor.Name),
		fmt.Sprintf("k8s-resource-%s-%s", monitor.Namespace, monitor.Name),
		fmt.Sprintf("k8s-uid-%s", monitor.UID),
	}
}

func (r *SakuraSimpleMonitorReconciler) setSyncSucceeded(ctx context.Context, monitor *monitoringv1alpha1.SakuraSimpleMonitor, monitorID string) error {
	now := metav1.NewTime(r.now())
	condition := metav1.Condition{
		Type:               conditionTypeSynced,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: monitor.Generation,
		Reason:             syncReasonSucceeded,
		Message:            "SakuraCloud simple monitor is synchronized",
	}

	return r.patchStatus(ctx, monitor, func(status *monitoringv1alpha1.SakuraSimpleMonitorStatus) {
		status.MonitorID = monitorID
		status.ObservedGeneration = monitor.Generation
		status.LastSyncedAt = &now
		meta.SetStatusCondition(&status.Conditions, condition)
	})
}

func (r *SakuraSimpleMonitorReconciler) setSyncFailed(ctx context.Context, monitor *monitoringv1alpha1.SakuraSimpleMonitor, syncErr error) error {
	condition := metav1.Condition{
		Type:               conditionTypeSynced,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: monitor.Generation,
		Reason:             syncReasonFailed,
		Message:            syncErr.Error(),
	}

	statusErr := r.patchStatus(ctx, monitor, func(status *monitoringv1alpha1.SakuraSimpleMonitorStatus) {
		status.ObservedGeneration = monitor.Generation
		meta.SetStatusCondition(&status.Conditions, condition)
	})
	if statusErr != nil {
		return errors.Join(syncErr, statusErr)
	}
	return syncErr
}

func (r *SakuraSimpleMonitorReconciler) patchStatus(
	ctx context.Context,
	monitor *monitoringv1alpha1.SakuraSimpleMonitor,
	mutate func(*monitoringv1alpha1.SakuraSimpleMonitorStatus),
) error {
	before := monitor.DeepCopy()
	mutate(&monitor.Status)
	return r.Status().Patch(ctx, monitor, client.MergeFrom(before))
}

func (r *SakuraSimpleMonitorReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}
