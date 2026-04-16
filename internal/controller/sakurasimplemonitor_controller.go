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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/azuki774/crd-sakura-simple-monitor/api/v1alpha1"
)

// SakuraSimpleMonitorReconciler reconciles a SakuraSimpleMonitor object
type SakuraSimpleMonitorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=monitoring.k8s.azuki.blue,resources=sakurasimplemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.k8s.azuki.blue,resources=sakurasimplemonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.k8s.azuki.blue,resources=sakurasimplemonitors/finalizers,verbs=update

// Reconcile currently validates that the resource can be fetched and logged.
// SakuraCloud API synchronization is implemented in a later step.
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

	logger.Info(
		"reconcile requested",
		"name", monitor.Name,
		"namespace", monitor.Namespace,
		"target", monitor.Spec.Target,
		"protocol", monitor.Spec.HealthCheck.Protocol,
	)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SakuraSimpleMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.SakuraSimpleMonitor{}).
		Named("sakurasimplemonitor").
		Complete(r)
}
