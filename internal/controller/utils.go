/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package controller

func Must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}
