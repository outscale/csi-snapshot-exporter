/*
Copyright 2025 Outscale SAS
All rights reserved.

Use of this source code is governed by a BSD 3 clause license
that can be found in the LICENSES/BSD-3-Clause.txt file.
*/
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/outscale/osc-sdk-go/v2"
	"k8s.io/klog/v2"
)

const maxResponseLength = 500

func clean(buf []byte) string {
	return strings.ReplaceAll(string(buf), `"`, ``)
}

func truncatedBody(httpResp *http.Response) string {
	body, err := io.ReadAll(httpResp.Body)
	if err == nil {
		str := []rune(clean(body))
		if len(str) > maxResponseLength {
			return string(str[:maxResponseLength/2]) + " [truncated] " + string(str[len(str)-maxResponseLength/2:])
		}
		return string(str)
	}
	return "(unable to fetch body)"
}

func errorBody(err error, httpResp *http.Response) (string, error) {
	body, rerr := io.ReadAll(httpResp.Body)
	if rerr == nil {
		return clean(body), extractOAPIError(err, body)
	}
	return "(unable to fetch body)", err
}

func LogAndExtractError(ctx context.Context, call string, request any, httpResp *http.Response, err error) error {
	logger := klog.FromContext(ctx).WithCallDepth(1)
	if logger.V(5).Enabled() {
		buf, _ := json.Marshal(request)
		logger.Info("OAPI request: "+clean(buf), "OAPI", call)
	}
	switch {
	case err != nil && httpResp == nil:
		logger.V(3).Error(err, "OAPI error", "OAPI", call)
	case httpResp == nil:
	case httpResp.StatusCode > 299:
		var body string
		body, err = errorBody(err, httpResp)
		err = fmt.Errorf("%s returned %w", call, err)
		logger.V(3).Info("OAPI error response: "+body, "OAPI", call, "http_status", httpResp.Status)
	case logger.V(5).Enabled(): // no error
		logger.Info("OAPI response: "+truncatedBody(httpResp), "OAPI", call)
	}
	return err
}

type OAPIError struct {
	errors []osc.Errors
}

func (err OAPIError) Error() string {
	if len(err.errors) == 0 {
		return "unknown error"
	}
	oe := err.errors[0]
	str := oe.GetCode() + "/" + oe.GetType()
	details := oe.GetDetails()
	if details != "" {
		str += " (" + details + ")"
	}
	return str
}

func extractOAPIError(err error, body []byte) error {
	var oerr osc.ErrorResponse
	jerr := json.Unmarshal(body, &oerr)
	if jerr == nil && len(*oerr.Errors) > 0 {
		return OAPIError{errors: *oerr.Errors}
	}
	return fmt.Errorf("http error: %w", err) //nolint
}
