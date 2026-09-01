package api

import "testing"

func TestParseDockerContainerRowsUsesFrontendContract(t *testing.T) {
	output := `{"ID":"abc123","Image":"star7th/showdoc:v3.9.2","Names":"WorkMesh-showdoc-demo","Ports":"0.0.0.0:4999->80/tcp, 443/tcp","State":"running","Status":"Up 1 minute","CreatedAt":"2026-09-01 00:00:00 +0800 CST","Labels":"com.docker.compose.project=showdoc,createdBy=Apps"}`
	items, err := parseDockerContainerRows(output, map[string]string{"WorkMesh-showdoc-demo": "showdoc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items=%d", len(items))
	}
	item := items[0]
	if item["containerID"] != "abc123" || item["name"] != "WorkMesh-showdoc-demo" || item["state"] != "running" {
		t.Fatalf("unexpected container item: %#v", item)
	}
	if item["isFromApp"] != true || item["isFromCompose"] != true || dockerLabel(item, "createdBy") != "Apps" {
		t.Fatalf("application association missing: %#v", item)
	}
}

func TestPaginateMapsUsesBoundedDefaults(t *testing.T) {
	items := []map[string]any{{"name": "one"}, {"name": "two"}}
	pageItems, page, pageSize := paginateMaps(items, 0, 0)
	if page != 1 || pageSize != 100 || len(pageItems) != 2 {
		t.Fatalf("page=%d pageSize=%d items=%d", page, pageSize, len(pageItems))
	}
}

func TestParseDockerStatsValues(t *testing.T) {
	if value := parseDockerPercent("12.5%"); value != 12.5 {
		t.Fatalf("percent=%v", value)
	}
	usage, limit := parseDockerMemoryUsage("64MiB / 2GiB")
	if usage != 64<<20 || limit != 2<<30 {
		t.Fatalf("usage=%d limit=%d", usage, limit)
	}
}
