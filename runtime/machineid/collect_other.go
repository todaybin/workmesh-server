// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

//go:build !linux && !windows

package machineid

func collect() (Components, error) { return Components{}, ErrUnavailable }
