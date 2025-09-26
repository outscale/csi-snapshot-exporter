/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package controller

import (
	"context"
	"fmt"

	"github.com/outscale/osc-sdk-go/v2"
)

//go:generate ../../bin/mockgen -destination mocks/oapi_mock.go -package mocks -source ./oapi.go
type OAPIClient interface {
	CreateSnapshotExportTask(ctx context.Context, req osc.CreateSnapshotExportTaskRequest) (*osc.SnapshotExportTask, error)
	ReadSnapshotExportTasks(ctx context.Context, req osc.ReadSnapshotExportTasksRequest) ([]osc.SnapshotExportTask, error)
}

type oapiClient struct {
	cfg  *osc.ConfigEnv
	oapi *osc.APIClient
}

func NewOAPIClient() (OAPIClient, error) {
	cfg := osc.NewConfigEnv()
	ccfg, err := cfg.Configuration()
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	ccfg.UserAgent = "csi-snapshot-exporter/" + Version
	_, err = cfg.Context(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("context: %w", err)
	}
	return &oapiClient{
		cfg:  cfg,
		oapi: osc.NewAPIClient(ccfg),
	}, nil
}

func (c *oapiClient) CreateSnapshotExportTask(ctx context.Context, req osc.CreateSnapshotExportTaskRequest) (*osc.SnapshotExportTask, error) {
	res, httpRes, err := c.oapi.SnapshotApi.CreateSnapshotExportTask(Must(c.cfg.Context(ctx))).CreateSnapshotExportTaskRequest(req).Execute()
	err = LogAndExtractError(ctx, "CreateSnapshotExportTask", req, httpRes, err)
	return res.SnapshotExportTask, err
}

func (c *oapiClient) ReadSnapshotExportTasks(ctx context.Context, req osc.ReadSnapshotExportTasksRequest) ([]osc.SnapshotExportTask, error) {
	res, httpRes, err := c.oapi.SnapshotApi.ReadSnapshotExportTasks(Must(c.cfg.Context(ctx))).ReadSnapshotExportTasksRequest(req).Execute()
	err = LogAndExtractError(ctx, "ReadSnapshotExportTasks", req, httpRes, err)
	return res.GetSnapshotExportTasks(), err
}
