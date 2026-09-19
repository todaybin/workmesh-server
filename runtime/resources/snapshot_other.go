// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

//go:build !linux

package resources

func collectPlatform() Snapshot { return Snapshot{} }
