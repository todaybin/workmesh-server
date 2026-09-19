// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package machineid 生成不暴露原始硬件字段的版本化机器指纹。
package machineid

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
)

const Version = 1

// ErrUnavailable 表示系统没有提供足以区分机器的稳定硬件字段。
var ErrUnavailable = errors.New("无法取得稳定机器硬件标识")

// Components 是生成机器码所需的规范化硬件字段。
type Components struct {
	ProductUUID   string
	BoardSerial   string
	ChassisSerial string
	PhysicalMACs  []string
	CPUSerial     string
	CPUIdentity   string
}

// Fingerprint 是可提交给 Gateway 的机器身份摘要。
type Fingerprint struct {
	Code    string
	Version int
}

// Current 采集当前机器硬件字段并计算指纹。
func Current() (Fingerprint, error) {
	components, err := collect()
	if err != nil {
		return Fingerprint{}, err
	}
	return FromComponents(components)
}

// FromComponents 使用稳定的字段顺序生成机器指纹，供平台实现和测试复用。
func FromComponents(components Components) (Fingerprint, error) {
	productUUID := normalize(components.ProductUUID)
	boardSerial := normalize(components.BoardSerial)
	chassisSerial := normalize(components.ChassisSerial)
	cpuSerial := normalize(components.CPUSerial)
	cpuIdentity := normalize(components.CPUIdentity)
	macs := normalizeMACs(components.PhysicalMACs)

	// 产品 UUID 或板级序列号提供机器锚点；物理网卡或 CPU Serial 提供第二个
	// 独立信号。只有 CPU 型号不能证明机器唯一，因此不计入最低条件。
	anchor := productUUID != "" || boardSerial != "" || chassisSerial != ""
	secondary := len(macs) > 0 || cpuSerial != ""
	if !anchor || !secondary {
		return Fingerprint{}, ErrUnavailable
	}
	canonical := strings.Join([]string{
		"workmesh-machine-v1",
		productUUID,
		boardSerial,
		chassisSerial,
		strings.Join(macs, ","),
		cpuSerial,
		cpuIdentity,
	}, "\n")
	sum := sha256.Sum256([]byte(canonical))
	return Fingerprint{Code: "sha256:" + hex.EncodeToString(sum[:]), Version: Version}, nil
}

func normalize(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func normalizeMACs(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", ":"))
		if value == "" || value == "00:00:00:00:00:00" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
