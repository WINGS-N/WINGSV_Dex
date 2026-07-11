package xraysubs

import (
	"encoding/base64"
	"testing"
)

func TestParseProfilesPlain(t *testing.T) {
	body := "vless://uuid@a.example:443#A\nvless://uuid@b.example:8443#B\n"
	nodes := ParseProfiles(body, "sub1", "Sub")
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if nodes[0].SubscriptionID != "sub1" || nodes[0].Title != "A" {
		t.Errorf("node0 = %+v", nodes[0])
	}
}

func TestParseProfilesBase64(t *testing.T) {
	inner := "vless://uuid@a.example:443#A\nvless://uuid@b.example:8443#B"
	body := base64.StdEncoding.EncodeToString([]byte(inner))
	nodes := ParseProfiles(body, "sub1", "Sub")
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes from base64 body, got %d", len(nodes))
	}
}

func TestParseProfilesDedupes(t *testing.T) {
	body := "vless://uuid@a.example:443#A\nvless://uuid@a.example:443#A\n"
	if n := ParseProfiles(body, "s", "S"); len(n) != 1 {
		t.Errorf("expected dedupe to 1, got %d", len(n))
	}
}

func TestParseQuota(t *testing.T) {
	q := ParseQuota("upload=100; download=200; total=1000; expire=1700000000")
	if q.Upload != 100 || q.Download != 200 || q.Total != 1000 || q.Expire != 1700000000 {
		t.Errorf("quota = %+v", q)
	}
}
