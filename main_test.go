package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-ping/ping"
)

func TestSourceURLForFamily(t *testing.T) {
	tests := []struct {
		name    string
		family  string
		want    string
		wantErr bool
	}{
		{name: "ipv4", family: "ipv4", want: ipv4SourceURL},
		{name: "ipv6", family: "ipv6", want: ipv6SourceURL},
		{name: "invalid", family: "other", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sourceURLForFamily(tt.family)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("sourceURLForFamily() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("sourceURLForFamily() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFetchTargets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"us-east-1":{"3.80.0.0/12":"3.80.0.0","52.0.0.0/15":"52.1.255.254"}},
			{"ap-northeast-1":{"3.112.0.0/14":"3.112.0.0"}}
		]`))
	}))
	defer server.Close()

	targets, err := fetchTargets(server.Client(), server.URL)
	if err != nil {
		t.Fatalf("fetchTargets() error = %v", err)
	}

	if len(targets) != 3 {
		t.Fatalf("fetchTargets() len = %d, want 3", len(targets))
	}

	if targets[0].Region != "ap-northeast-1" || targets[0].Prefix != "3.112.0.0/14" {
		t.Fatalf("unexpected first target: %+v", targets[0])
	}

	if targets[2].Region != "us-east-1" || targets[2].Prefix != "52.0.0.0/15" {
		t.Fatalf("unexpected last target: %+v", targets[2])
	}
}

func TestFilterTargets(t *testing.T) {
	targets := []Target{
		{Region: "ap-northeast-1", Address: "3.112.0.0"},
		{Region: "us-east-1", Address: "3.80.0.0"},
		{Region: "ap-southeast-1", Address: "3.0.0.9"},
	}

	filtered := filterTargets(targets, []string{"ap-northeast-1", "us-east-1"})
	if len(filtered) != 2 {
		t.Fatalf("filterTargets() len = %d, want 2", len(filtered))
	}

	if filtered[0].Region != "ap-northeast-1" || filtered[1].Region != "us-east-1" {
		t.Fatalf("unexpected filtered targets: %+v", filtered)
	}
}

func TestParseRegions(t *testing.T) {
	regions := parseRegions(" ap-northeast-1,us-east-1,ap-northeast-1 ,, ")
	if len(regions) != 2 {
		t.Fatalf("parseRegions() len = %d, want 2", len(regions))
	}

	if regions[0] != "ap-northeast-1" || regions[1] != "us-east-1" {
		t.Fatalf("unexpected regions: %+v", regions)
	}
}

func TestSortResults(t *testing.T) {
	results := []Result{
		{
			Target: Target{Region: "us-east-1", Address: "52.1.255.254"},
			Stats:  &ping.Statistics{PacketsRecv: 1, AvgRtt: 120 * time.Millisecond, PacketLoss: 0},
		},
		{
			Target: Target{Region: "ap-northeast-1", Address: "3.112.0.0"},
			Stats:  &ping.Statistics{PacketsRecv: 1, AvgRtt: 40 * time.Millisecond, PacketLoss: 0},
		},
		{
			Target: Target{Region: "ap-southeast-1", Address: "3.0.0.9"},
			Stats:  &ping.Statistics{PacketsRecv: 0, PacketLoss: 100},
		},
	}

	sortResults(results)

	if results[0].Target.Region != "ap-northeast-1" {
		t.Fatalf("expected fastest reachable target first, got %+v", results[0])
	}

	if results[2].Target.Region != "ap-southeast-1" {
		t.Fatalf("expected unreachable target last, got %+v", results[2])
	}
}
