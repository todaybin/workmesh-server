//go:build !linux

// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package taskruntime

import (
	"context"
	"errors"
)

func newCgroupV2Controller(string, string) (*CgroupV2Controller, error) {
	return nil, errors.New("当前平台不支持 Linux cgroup v2")
}

func (*CgroupV2Controller) create(context.Context, string, ResourceLimits) (CgroupV2Handle, error) {
	return CgroupV2Handle{}, errors.New("当前平台不支持 Linux cgroup v2")
}

func (*CgroupV2Controller) attach(context.Context, CgroupV2Handle, int) error {
	return errors.New("当前平台不支持 Linux cgroup v2")
}

func (*CgroupV2Controller) destroy(context.Context, CgroupV2Handle) error {
	return errors.New("当前平台不支持 Linux cgroup v2")
}
