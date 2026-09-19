// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

//go:build windows

package machineid

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

func collect() (Components, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	script := `$cs=Get-CimInstance Win32_ComputerSystemProduct|Select-Object -First 1 UUID;` +
		`$bb=Get-CimInstance Win32_BaseBoard|Select-Object -First 1 SerialNumber;` +
		`$ch=Get-CimInstance Win32_SystemEnclosure|Select-Object -First 1 SerialNumber;` +
		`$cp=Get-CimInstance Win32_Processor|Select-Object -First 1 ProcessorId,Manufacturer,Name;` +
		`$mac=@(Get-NetAdapter -Physical|Where-Object {$_.MacAddress}|ForEach-Object {$_.MacAddress});` +
		`@{ProductUUID=$cs.UUID;BoardSerial=$bb.SerialNumber;ChassisSerial=$ch.SerialNumber;CPUSerial=$cp.ProcessorId;CPUIdentity=($cp.Manufacturer+'|'+$cp.Name);PhysicalMACs=$mac}|ConvertTo-Json -Compress`
	output, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return Components{}, ErrUnavailable
	}
	var raw struct {
		ProductUUID, BoardSerial, ChassisSerial, CPUSerial, CPUIdentity string
		PhysicalMACs                                                    any
	}
	if json.Unmarshal(output, &raw) != nil {
		return Components{}, ErrUnavailable
	}
	macs := make([]string, 0)
	switch value := raw.PhysicalMACs.(type) {
	case string:
		macs = append(macs, value)
	case []any:
		for _, item := range value {
			if text, ok := item.(string); ok {
				macs = append(macs, strings.TrimSpace(text))
			}
		}
	}
	return Components{ProductUUID: raw.ProductUUID, BoardSerial: raw.BoardSerial, ChassisSerial: raw.ChassisSerial, CPUSerial: raw.CPUSerial, CPUIdentity: raw.CPUIdentity, PhysicalMACs: macs}, nil
}
