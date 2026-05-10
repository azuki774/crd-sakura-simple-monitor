package controller

import (
	"context"
	"errors"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	monitoringv1alpha1 "github.com/azuki774/crd-sakura-simple-monitor/api/v1alpha1"
	"github.com/azuki774/crd-sakura-simple-monitor/internal/simplemonitor"
)

const (
	sakuraSimpleMonitorFinalizer = "sakurasimplemonitor.monitoring.k8s.azuki.blue/finalizer"

	conditionTypeSynced = "Synced"

	syncReasonSucceeded    = "SyncSucceeded"
	syncReasonFailed       = "SyncFailed"
	syncReasonDeleteFailed = "DeleteFailed"

	syncVerificationInterval = 24 * time.Hour
	deleteRetryInterval      = 4 * time.Hour
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
		return r.reconcileDelete(ctx, monitor)
	}

	if err := r.ensureFinalizer(ctx, monitor); err != nil {
		return ctrl.Result{}, err
	}

	desired := desiredSimpleMonitor(monitor)
	if monitor.Status.ObservedGeneration == monitor.Generation {
		return r.reconcileProcessedGeneration(ctx, monitor, desired)
	}

	if r.SakuraSimpleMonitor == nil {
		err := errors.New("sakura simple monitor client is not configured")
		return ctrl.Result{}, r.setSyncFailed(ctx, monitor, monitor.Status.MonitorID, err)
	}

	var monitorID string
	var err error

	if monitor.Status.MonitorID == "" {
		monitorID, err = r.SakuraSimpleMonitor.Create(ctx, desired)
	} else {
		monitorID = monitor.Status.MonitorID
		err = r.SakuraSimpleMonitor.Update(ctx, monitorID, desired)
		if errors.Is(err, simplemonitor.ErrSimpleMonitorNotFound) {
			monitorID, err = r.SakuraSimpleMonitor.Create(ctx, desired)
		}
	}

	if err != nil {
		logger.Error(err, "failed to synchronize SakuraCloud simple monitor", "monitorID", monitor.Status.MonitorID)
		return ctrl.Result{}, r.setSyncFailed(ctx, monitor, monitorID, err)
	}

	if monitorID == "" {
		err = errors.New("sakura simple monitor API returned an empty monitor ID")
		return ctrl.Result{}, r.setSyncFailed(ctx, monitor, monitorID, err)
	}

	logger.Info(
		"synchronized SakuraCloud simple monitor",
		"name", monitor.Name,
		"namespace", monitor.Namespace,
		"target", monitor.Spec.Target,
		"protocol", monitor.Spec.HealthCheck.Protocol,
		"monitorID", monitorID,
	)

	return ctrl.Result{RequeueAfter: syncVerificationInterval}, r.setSyncSucceeded(ctx, monitor, monitorID)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SakuraSimpleMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.SakuraSimpleMonitor{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{},
			deleteTimestampChangedPredicate{},
		))).
		Named("sakurasimplemonitor").
		Complete(r)
}

func (r *SakuraSimpleMonitorReconciler) ensureFinalizer(
	ctx context.Context,
	monitor *monitoringv1alpha1.SakuraSimpleMonitor,
) error {
	if controllerutil.ContainsFinalizer(monitor, sakuraSimpleMonitorFinalizer) {
		return nil
	}

	before := monitor.DeepCopy()
	controllerutil.AddFinalizer(monitor, sakuraSimpleMonitorFinalizer)
	return r.Patch(ctx, monitor, client.MergeFrom(before))
}

func (r *SakuraSimpleMonitorReconciler) reconcileDelete(
	ctx context.Context,
	monitor *monitoringv1alpha1.SakuraSimpleMonitor,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(monitor, sakuraSimpleMonitorFinalizer) {
		return ctrl.Result{}, nil
	}

	monitorID := monitor.Status.MonitorID
	if monitorID == "" {
		logger.Info("skip deleting SakuraCloud simple monitor because monitorID is empty")
		return ctrl.Result{}, r.removeFinalizer(ctx, monitor)
	}

	if r.SakuraSimpleMonitor == nil {
		err := errors.New("sakura simple monitor client is not configured")
		logger.Error(err, "failed to delete SakuraCloud simple monitor", "monitorID", monitorID)
		if statusErr := r.setDeleteFailed(ctx, monitor, err); statusErr != nil {
			logger.Error(statusErr, "failed to record SakuraCloud simple monitor deletion failure", "monitorID", monitorID)
		}
		return ctrl.Result{RequeueAfter: deleteRetryInterval}, nil
	}

	if err := r.SakuraSimpleMonitor.Delete(ctx, monitorID); err != nil {
		if errors.Is(err, simplemonitor.ErrSimpleMonitorNotFound) {
			logger.Info("SakuraCloud simple monitor is already deleted", "monitorID", monitorID)
			return ctrl.Result{}, r.removeFinalizer(ctx, monitor)
		}

		logger.Error(err, "failed to delete SakuraCloud simple monitor", "monitorID", monitorID)
		if statusErr := r.setDeleteFailed(ctx, monitor, err); statusErr != nil {
			logger.Error(statusErr, "failed to record SakuraCloud simple monitor deletion failure", "monitorID", monitorID)
		}
		return ctrl.Result{RequeueAfter: deleteRetryInterval}, nil
	}

	logger.Info("deleted SakuraCloud simple monitor for deleting resource", "monitorID", monitorID)
	return ctrl.Result{}, r.removeFinalizer(ctx, monitor)
}

func (r *SakuraSimpleMonitorReconciler) removeFinalizer(
	ctx context.Context,
	monitor *monitoringv1alpha1.SakuraSimpleMonitor,
) error {
	before := monitor.DeepCopy()
	controllerutil.RemoveFinalizer(monitor, sakuraSimpleMonitorFinalizer)
	return r.Patch(ctx, monitor, client.MergeFrom(before))
}

type deleteTimestampChangedPredicate struct {
	predicate.Funcs
}

func (deleteTimestampChangedPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}
	return e.ObjectOld.GetDeletionTimestamp().IsZero() && !e.ObjectNew.GetDeletionTimestamp().IsZero()
}

func desiredSimpleMonitor(monitor *monitoringv1alpha1.SakuraSimpleMonitor) simplemonitor.SimpleMonitorDesired {
	return simplemonitor.SimpleMonitorDesired{
		Target:         monitor.Spec.Target,
		Description:    monitor.Spec.Description,
		Protocol:       monitor.Spec.HealthCheck.Protocol,
		Port:           monitor.Spec.HealthCheck.Port,
		Path:           monitor.Spec.HealthCheck.Path,
		ExpectedStatus: monitor.Spec.HealthCheck.ExpectedStatus,
		TimeoutSeconds: monitor.Spec.HealthCheck.TimeoutSeconds,
		HTTP2:          monitor.Spec.HealthCheck.HTTP2,
		Interval:       monitor.Spec.Interval,
		RetryInterval:  monitor.Spec.RetryInterval,
		WebhookURL:     monitor.Spec.Notifications.WebhookURL,
		RepeatInterval: monitor.Spec.Notifications.RepeatInterval,
	}
}

func (r *SakuraSimpleMonitorReconciler) reconcileProcessedGeneration(
	ctx context.Context,
	monitor *monitoringv1alpha1.SakuraSimpleMonitor,
	desired simplemonitor.SimpleMonitorDesired,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if !isSyncSucceeded(monitor) {
		logger.Info(
			"SakuraCloud simple monitor is already failed for this generation",
			"name", monitor.Name,
			"namespace", monitor.Namespace,
			"generation", monitor.Generation,
			"monitorID", monitor.Status.MonitorID,
		)
		return ctrl.Result{}, nil
	}

	if remaining := r.syncVerificationRemaining(monitor); remaining > 0 {
		logger.Info(
			"SakuraCloud simple monitor synchronization verification is not due",
			"name", monitor.Name,
			"namespace", monitor.Namespace,
			"generation", monitor.Generation,
			"monitorID", monitor.Status.MonitorID,
			"requeueAfter", remaining,
		)
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	if r.SakuraSimpleMonitor == nil {
		err := errors.New("sakura simple monitor client is not configured")
		return ctrl.Result{}, r.setSyncFailed(ctx, monitor, monitor.Status.MonitorID, err)
	}

	if monitor.Status.MonitorID == "" {
		err := errors.New("sakura simple monitor ID is empty during synchronization verification")
		return ctrl.Result{}, r.setSyncFailed(ctx, monitor, monitor.Status.MonitorID, err)
	}

	if err := r.SakuraSimpleMonitor.CheckSynced(ctx, monitor.Status.MonitorID, desired); err != nil {
		logger.Error(err, "failed to verify SakuraCloud simple monitor synchronization", "monitorID", monitor.Status.MonitorID)
		return ctrl.Result{}, r.setSyncFailed(ctx, monitor, monitor.Status.MonitorID, err)
	}

	logger.Info(
		"verified SakuraCloud simple monitor synchronization",
		"name", monitor.Name,
		"namespace", monitor.Namespace,
		"generation", monitor.Generation,
		"monitorID", monitor.Status.MonitorID,
	)
	return ctrl.Result{RequeueAfter: syncVerificationInterval}, r.setSyncSucceeded(ctx, monitor, monitor.Status.MonitorID)
}

func isSyncSucceeded(monitor *monitoringv1alpha1.SakuraSimpleMonitor) bool {
	condition := meta.FindStatusCondition(monitor.Status.Conditions, conditionTypeSynced)
	return condition != nil && condition.Status == metav1.ConditionTrue
}

func (r *SakuraSimpleMonitorReconciler) syncVerificationRemaining(monitor *monitoringv1alpha1.SakuraSimpleMonitor) time.Duration {
	if monitor.Status.LastSyncedAt == nil {
		return 0
	}
	remaining := syncVerificationInterval - r.now().Sub(monitor.Status.LastSyncedAt.Time)
	if remaining <= 0 {
		return 0
	}
	return remaining
}

func (r *SakuraSimpleMonitorReconciler) setSyncSucceeded(
	ctx context.Context,
	monitor *monitoringv1alpha1.SakuraSimpleMonitor,
	monitorID string,
) error {
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

func (r *SakuraSimpleMonitorReconciler) setSyncFailed(
	ctx context.Context,
	monitor *monitoringv1alpha1.SakuraSimpleMonitor,
	monitorID string,
	syncErr error,
) error {
	condition := metav1.Condition{
		Type:               conditionTypeSynced,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: monitor.Generation,
		Reason:             syncReasonFailed,
		Message:            syncErr.Error(),
	}

	return r.patchStatus(ctx, monitor, func(status *monitoringv1alpha1.SakuraSimpleMonitorStatus) {
		if monitorID != "" {
			status.MonitorID = monitorID
		}
		status.ObservedGeneration = monitor.Generation
		meta.SetStatusCondition(&status.Conditions, condition)
	})
}

func (r *SakuraSimpleMonitorReconciler) setDeleteFailed(
	ctx context.Context,
	monitor *monitoringv1alpha1.SakuraSimpleMonitor,
	deleteErr error,
) error {
	condition := metav1.Condition{
		Type:               conditionTypeSynced,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: monitor.Generation,
		Reason:             syncReasonDeleteFailed,
		Message:            deleteErr.Error(),
	}

	return r.patchStatus(ctx, monitor, func(status *monitoringv1alpha1.SakuraSimpleMonitorStatus) {
		status.ObservedGeneration = monitor.Generation
		meta.SetStatusCondition(&status.Conditions, condition)
	})
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
