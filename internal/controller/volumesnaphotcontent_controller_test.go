/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package controller_test

import (
	"context"
	"testing"
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/outscale/csi-snapshot-exporter/internal/controller"
	"github.com/outscale/csi-snapshot-exporter/internal/controller/mocks"
	"github.com/outscale/osc-sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func initTest(mockCtl *gomock.Controller, vsc *snapshotv1.VolumeSnapshotContent, class *snapshotv1.VolumeSnapshotClass) (*controller.VolumeSnaphotContentReconciler, *mocks.MockOAPIClient) {
	fakeScheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(fakeScheme)
	_ = snapshotv1.AddToScheme(fakeScheme)
	client := fake.NewClientBuilder().WithScheme(fakeScheme).
		WithStatusSubresource(vsc).WithObjects(vsc, class).Build()
	oapi := mocks.NewMockOAPIClient(mockCtl)
	return controller.NewVolumeSnaphotContentReconciler(client, fakeScheme, oapi), oapi
}

func TestReconcile(t *testing.T) {
	class := &snapshotv1.VolumeSnapshotClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "VolumeSnapshotClass",
			APIVersion: "snapshot.storage.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "vsclass",
		},
		Parameters: map[string]string{
			controller.ParamExportEnabled: "true",
			controller.ParamExportBucket:  "bucket",
			controller.ParamExportPrefix:  "/{vs}/{ns}/{date}",
		},
	}
	vsc := &snapshotv1.VolumeSnapshotContent{
		TypeMeta: metav1.TypeMeta{
			Kind:       "VolumeSnapshotContent",
			APIVersion: "snapshot.storage.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "vsc",
		},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			VolumeSnapshotRef: corev1.ObjectReference{
				Name:      "vs",
				Namespace: "ns",
			},
			VolumeSnapshotClassName: &class.Name,
		},
		Status: &snapshotv1.VolumeSnapshotContentStatus{
			SnapshotHandle: ptr.To("snap-foo"),
		},
	}
	req := controllerruntime.Request{
		NamespacedName: types.NamespacedName{
			Name: "vsc",
		},
	}
	t.Run("No export is done if export is not enabled", func(t *testing.T) {
		class := class.DeepCopy()
		delete(class.Parameters, controller.ParamExportEnabled)
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()
		r, _ := initTest(mockCtl, vsc, class)
		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		assert.Zero(t, res.RequeueAfter)
	})
	t.Run("Request is requeued if snapshot is not available", func(t *testing.T) {
		vsc := vsc.DeepCopy()
		vsc.Status.SnapshotHandle = nil
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()
		r, _ := initTest(mockCtl, vsc, class)
		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		assert.NotZero(t, res.RequeueAfter)
	})
	t.Run("An export is started", func(t *testing.T) {
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()
		r, mockOAPI := initTest(mockCtl, vsc, class)
		mockOAPI.EXPECT().CreateSnapshotExportTask(gomock.Any(), gomock.Eq(osc.CreateSnapshotExportTaskRequest{
			SnapshotId: "snap-foo",
			OsuExport: osc.OsuExportToCreate{
				DiskImageFormat: "qcow2",
				OsuBucket:       "bucket",
				OsuPrefix:       ptr.To("/vs/ns/" + time.Now().Format(time.DateOnly)),
			},
		})).
			Return(&osc.SnapshotExportTask{
				State: ptr.To(controller.StatePending),
			}, nil)
		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		assert.NotZero(t, res.RequeueAfter)
	})
	t.Run("Reconciliation continues when the export is not completed", func(t *testing.T) {
		vsc := vsc.DeepCopy()
		vsc.Annotations = map[string]string{
			controller.AnnotationExportTask:  "snap-export-foo",
			controller.AnnotationExportState: controller.StatePending,
		}
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()
		r, mockOAPI := initTest(mockCtl, vsc, class)
		mockOAPI.EXPECT().ReadSnapshotExportTasks(gomock.Any(), gomock.Eq(osc.ReadSnapshotExportTasksRequest{
			Filters: &osc.FiltersExportTask{
				TaskIds: &[]string{"snap-export-foo"},
			},
		})).
			Return([]osc.SnapshotExportTask{{
				State: ptr.To(string(controller.StateActive)),
			}}, nil)
		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		assert.NotZero(t, res.RequeueAfter)
	})
	t.Run("Reconciliation finishes when the export is completed", func(t *testing.T) {
		vsc := vsc.DeepCopy()
		vsc.Annotations = map[string]string{
			controller.AnnotationExportTask:  "snap-export-foo",
			controller.AnnotationExportState: controller.StatePending,
		}
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()
		r, mockOAPI := initTest(mockCtl, vsc, class)
		mockOAPI.EXPECT().ReadSnapshotExportTasks(gomock.Any(), gomock.Eq(osc.ReadSnapshotExportTasksRequest{
			Filters: &osc.FiltersExportTask{
				TaskIds: &[]string{"snap-export-foo"},
			},
		})).
			Return([]osc.SnapshotExportTask{{
				State: ptr.To(string(controller.StateCompleted)),
			}}, nil)
		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		assert.Zero(t, res.RequeueAfter)
	})
	t.Run("Request is requeued when the export has failed", func(t *testing.T) {
		vsc := vsc.DeepCopy()
		vsc.Annotations = map[string]string{
			controller.AnnotationExportTask:  "snap-export-foo",
			controller.AnnotationExportState: controller.StateActive,
		}
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()
		r, mockOAPI := initTest(mockCtl, vsc, class)
		mockOAPI.EXPECT().ReadSnapshotExportTasks(gomock.Any(), gomock.Eq(osc.ReadSnapshotExportTasksRequest{
			Filters: &osc.FiltersExportTask{
				TaskIds: &[]string{"snap-export-foo"},
			},
		})).
			Return([]osc.SnapshotExportTask{{
				State: ptr.To(string(controller.StateFailed)),
			}}, nil)
		mockOAPI.EXPECT().CreateSnapshotExportTask(gomock.Any(), gomock.Eq(osc.CreateSnapshotExportTaskRequest{
			SnapshotId: "snap-foo",
			OsuExport: osc.OsuExportToCreate{
				DiskImageFormat: "qcow2",
				OsuBucket:       "bucket",
				OsuPrefix:       ptr.To("/vs/ns/" + time.Now().Format(time.DateOnly)),
			},
		})).
			Return(&osc.SnapshotExportTask{
				State: ptr.To(controller.StatePending),
			}, nil)
		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		assert.NotZero(t, res.RequeueAfter)
	})
}
