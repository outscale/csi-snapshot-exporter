/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package controller

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ParamExportEnabled = "exportToOOS"
	ParamExportFormat  = "exportImageFormat"
	ParamExportBucket  = "exportBucket"
	ParamExportPrefix  = "exportPrefix"

	//
	AnnotationExportPath  = "bsu.csi.outscale.com/export-path"
	AnnotationExportState = "bsu.csi.outscale.com/export-state"
	AnnotationExportTask  = "bsu.csi.outscale.com/export-task"
)

type Scope struct {
	client     client.Client
	snapBefore runtime.Object

	snap      *volumesnapshotv1.VolumeSnapshotContent
	snapClass *volumesnapshotv1.VolumeSnapshotClass
}

// NewScope create new clusterScope from parameters which is called at each reconciliation iteration
func NewScope(c client.Client, snap *volumesnapshotv1.VolumeSnapshotContent, snapClass *volumesnapshotv1.VolumeSnapshotClass) *Scope {
	return &Scope{
		client:     c,
		snapBefore: snap.DeepCopyObject(),
		snap:       snap,
		snapClass:  snapClass,
	}
}

func (s *Scope) NeedsExport() bool {
	return s.snapClass.Parameters[ParamExportEnabled] == "true" && s.snap.Annotations[AnnotationExportState] != string(osc.SnapshotExportTaskStateCompleted)
}

func (s *Scope) GetSnapshotID() (string, bool) {
	if s.snap.Status == nil || s.snap.Status.SnapshotHandle == nil {
		return "", false
	}
	return *s.snap.Status.SnapshotHandle, true
}

func (s *Scope) ExportTaskID() string {
	return s.snap.Annotations[AnnotationExportTask]
}

func (s *Scope) ExportBucket() string {
	return s.snapClass.Parameters[ParamExportBucket]
}

func (s *Scope) ExportPrefix() string {
	prefix := s.snapClass.Parameters[ParamExportPrefix]
	if strings.Contains(prefix, "{") {
		prefix = strings.NewReplacer(
			"{date}", time.Now().Format(time.DateOnly),
			"{vs}", s.snap.Spec.VolumeSnapshotRef.Name,
			"{ns}", s.snap.Spec.VolumeSnapshotRef.Namespace,
		).Replace(prefix)
	}
	return prefix
}

func (s *Scope) ExportFormat() (string, error) {
	f := s.snapClass.Parameters[ParamExportFormat]
	switch f {
	case "":
		return "qcow2", nil
	case "qcow2", "raw":
		return f, nil
	default:
		return "", fmt.Errorf("invalid format %q - allowed values qcow2,raw", f)
	}
}

func (s *Scope) UpdateExportState(id string, state osc.SnapshotExportTaskState) {
	if s.snap.Annotations == nil {
		s.snap.Annotations = map[string]string{}
	}
	s.snap.Annotations[AnnotationExportTask] = id
	s.snap.Annotations[AnnotationExportState] = string(state)
}

func (s *Scope) SetExportPath(path string) {
	s.snap.Annotations[AnnotationExportPath] = path
}

// Close closes the scope of the cluster configuration and status
func (s *Scope) Close(ctx context.Context) error {
	before, err := runtime.DefaultUnstructuredConverter.ToUnstructured(s.snapBefore)
	if err != nil {
		return err
	}
	after, err := runtime.DefaultUnstructuredConverter.ToUnstructured(s.snap)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(before, after) {
		return nil
	}

	patch := client.MergeFrom(s.snapBefore.(client.Object))
	if err := s.client.Patch(ctx, s.snap, patch); err != nil {
		return fmt.Errorf("patch: %w", err)
	}
	return nil
}
