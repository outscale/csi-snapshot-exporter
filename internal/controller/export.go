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

	"github.com/outscale/osc-sdk-go/v2"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *VolumeSnaphotContentReconciler) export(ctx context.Context, scope *Scope) (ctrl.Result, error) {
	log := klog.FromContext(ctx)
	var task *osc.SnapshotExportTask
	if taskID := scope.ExportTaskID(); taskID != "" {
		tasks, err := r.oapi.ReadSnapshotExportTasks(ctx, osc.ReadSnapshotExportTasksRequest{
			Filters: &osc.FiltersExportTask{TaskIds: &[]string{taskID}},
		})
		switch {
		case err != nil:
			return ctrl.Result{}, fmt.Errorf("unable to read task: %w", err)
		case len(tasks) == 0:
			return ctrl.Result{}, errors.New("no export task found")
		}
		task = &tasks[0]
		if task.GetState() == StateFailed {
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
		task, err = r.oapi.CreateSnapshotExportTask(ctx, req)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to create task: %w", err)
		}
		log.V(2).Info("New export task created", "task_id", task.GetTaskId())
	}
	scope.UpdateExportState(task.GetTaskId(), task.GetState())
	switch task.GetState() {
	case StateCompleted:
		path := task.OsuExport.GetOsuPrefix() + task.GetSnapshotId() +
			strings.TrimPrefix(task.GetTaskId(), "snap-export") + "." + task.OsuExport.GetDiskImageFormat() + ".gz"
		scope.SetExportPath(path)
		log.V(2).Info("Export is finished", "task_id", task.GetTaskId(), "state", task.GetState(), "path", path)
		return ctrl.Result{}, nil
	case StateCancelled:
		log.V(2).Info("Export was cancelled", "task_id", task.GetTaskId(), "state", task.GetState())
		return ctrl.Result{}, nil
	case StateFailed:
		log.V(3).Info("Export has failed, retrying", "task_id", task.GetTaskId(), "state", task.GetState())
		return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
	}
	log.V(4).Info("Export is still running", "task_id", task.GetTaskId(), "state", task.GetState(), "progress", task.GetProgress())
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}
