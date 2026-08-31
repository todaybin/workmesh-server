// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import "testing"

func TestParseListeningOutput(t *testing.T) {
	output := "Netid State Local Address:Port Peer Address:Port Process\n" +
		"tcp LISTEN 0 128 127.0.0.1:9999 0.0.0.0:* users:((workmesh,pid=10,fd=3))\n" +
		"udp UNCONN 0 0 0.0.0.0:53 0.0.0.0:* users:((dns,pid=11,fd=4))\n"
	items := parseListeningOutput(output)
	if len(items) != 2 {
		t.Fatalf("expected two listening entries, got %#v", items)
	}
	if items[0]["protocol"] != "tcp" || items[0]["localAddress"] != "127.0.0.1:9999" {
		t.Fatalf("unexpected first entry: %#v", items[0])
	}
	if items[0]["process"] == "" {
		t.Fatal("process metadata should be preserved")
	}
}

func TestParseListeningOutputIsBounded(t *testing.T) {
	output := "header\n"
	for i := 0; i < 1100; i++ {
		output += "tcp LISTEN 0 128 127.0.0.1:1 0.0.0.0:*\n"
	}
	if got := len(parseListeningOutput(output)); got != 1024 {
		t.Fatalf("expected 1024-entry bound, got %d", got)
	}
}
