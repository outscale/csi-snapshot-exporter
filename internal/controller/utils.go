/*
Copyright 2025 Outscale SAS
All rights reserved.

Use of this source code is governed by a BSD 3 clause license
that can be found in the LICENSES/BSD-3-Clause.txt file.
*/
package controller

func Must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}
