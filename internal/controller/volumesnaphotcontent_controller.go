/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package controller

import (
	"context"
	"fmt"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// VolumeSnaphotContentReconciler reconciles a VolumeSnaphotContent object
type VolumeSnaphotContentReconciler struct {
	k8s    client.Client
	oapi   osc.ClientInterface
	Scheme *runtime.Scheme
}

func NewVolumeSnaphotContentReconciler(k8s client.Client, scheme *runtime.Scheme, oapi osc.ClientInterface) *VolumeSnaphotContentReconciler {
	return &VolumeSnaphotContentReconciler{
		k8s:    k8s,
		oapi:   oapi,
		Scheme: scheme,
	}
}

// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshotcontents,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshotclasses,verbs=get;list;watch

func (r *VolumeSnaphotContentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := klog.FromContext(ctx)

	var snap volumesnapshotv1.VolumeSnapshotContent
	if err := r.k8s.Get(ctx, req.NamespacedName, &snap); err != nil {
		err = fmt.Errorf("unable to fetch snapshot: %w", err)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !snap.DeletionTimestamp.IsZero() {
		log.V(3).Info("Snaphot is being deleted")
		return ctrl.Result{}, nil
	}
	if snap.Spec.VolumeSnapshotClassName == nil {
		log.V(3).Info("Snaphot has no class")
		return ctrl.Result{}, nil
	}

	var snapClass volumesnapshotv1.VolumeSnapshotClass
	if err := r.k8s.Get(ctx, types.NamespacedName{Name: *snap.Spec.VolumeSnapshotClassName}, &snapClass); err != nil {
		err = fmt.Errorf("unable to fetch snapshot class: %w", err)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	scope := NewScope(r.k8s, &snap, &snapClass)
	if !scope.NeedsExport() {
		log.V(3).Info("No need to export snapshot")
		return ctrl.Result{}, nil
	}
	defer func() {
		if err := scope.Close(ctx); reterr == nil {
			reterr = err
		}
	}()
	return r.export(ctx, scope)
}

// SetupWithManager sets up the controller with the Manager.
func (r *VolumeSnaphotContentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&volumesnapshotv1.VolumeSnapshotContent{}).
		Named("snapshot_exporter").
		Complete(r)
}
