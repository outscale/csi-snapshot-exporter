/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/outscale/goutils/sdk/ptr"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *VolumeSnaphotContentReconciler) export(ctx context.Context, scope *Scope) (ctrl.Result, error) {
	log := klog.FromContext(ctx)
	var task *osc.SnapshotExportTask
	if taskID := scope.ExportTaskID(); taskID != "" {
		res, err := r.oapi.ReadSnapshotExportTasks(ctx, osc.ReadSnapshotExportTasksRequest{
			Filters: &osc.FiltersExportTask{TaskIds: &[]string{taskID}},
		})
		switch {
		case err != nil:
			return ctrl.Result{}, fmt.Errorf("unable to read task: %w", err)
		case len(*res.SnapshotExportTasks) == 0:
			return ctrl.Result{}, errors.New("no export task found")
		}
		task = &(*res.SnapshotExportTasks)[0]
		if task.State == osc.SnapshotExportTaskStateFailed {
			log.V(3).Info("Retrying failed export")
			task = nil
		}
	}
	if task == nil {
		f, err := scope.ExportFormat()
		if err != nil {
			log.V(2).Error(err, "Unable to export snapshot")
			return ctrl.Result{}, nil
		}
		b := scope.ExportBucket()
		if b == "" {
			log.V(2).Error(errors.New("bucket is required"), "Unable to export snapshot")
			return ctrl.Result{}, nil
		}
		id, found := scope.GetSnapshotID()
		if !found {
			log.V(4).Info("Snapshot does not exist yet")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		req := osc.CreateSnapshotExportTaskRequest{
			SnapshotId: id,
			OsuExport: osc.OsuExportToCreate{
				DiskImageFormat: f,
				OsuBucket:       b,
			},
		}
		if p := scope.ExportPrefix(); p != "" {
			req.OsuExport.OsuPrefix = &p
		}
		res, err := r.oapi.CreateSnapshotExportTask(ctx, req)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to create task: %w", err)
		}
		task = res.SnapshotExportTask
		log.V(2).Info("New export task created", "task_id", task.TaskId)
	}
	scope.UpdateExportState(task.TaskId, task.State)
	switch task.State {
	case osc.SnapshotExportTaskStateCompleted:
		path := ptr.From(task.OsuExport.OsuPrefix) + task.SnapshotId +
			strings.TrimPrefix(task.TaskId, "snap-export") + "." + task.OsuExport.DiskImageFormat + ".gz"
		scope.SetExportPath(path)
		log.V(2).Info("Export is finished", "task_id", task.TaskId, "state", task.State, "path", path)
		return ctrl.Result{}, nil
	case osc.SnapshotExportTaskStateCancelled:
		log.V(2).Info("Export was cancelled", "task_id", task.TaskId, "state", task.State)
		return ctrl.Result{}, nil
	case osc.SnapshotExportTaskStateFailed:
		log.V(3).Info("Export has failed, retrying", "task_id", task.TaskId, "state", task.State)
		return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
	}
	log.V(4).Info("Export is still running", "task_id", task.TaskId, "state", task.State, "progress", task.Progress)
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}
