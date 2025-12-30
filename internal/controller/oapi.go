/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package controller

import (
	"context"

	"github.com/outscale/goutils/k8s/sdk"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
)

func NewOAPIClient(ctx context.Context, opts sdk.Options) (osc.ClientInterface, error) {
	ua := "csi-snapshot-exporter/" + Version
	_, c, err := sdk.NewSDKClient(ctx, ua, opts)
	return c, err
}
