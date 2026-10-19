package plugin

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultRetentionDays(t *testing.T) {
	config, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if config.RetentionDays != 365 {
		t.Fatalf("default retention days = %d, want 365", config.RetentionDays)
	}
	if config.SessionTTLMinutes != defaultSessionTTLMinutes {
		t.Fatalf("default session TTL = %d minutes, want %d", config.SessionTTLMinutes, defaultSessionTTLMinutes)
	}
}

func TestSessionTTLConfig(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    int
		wantErr bool
	}{
		{name: "default", want: defaultSessionTTLMinutes},
		{name: "custom", yaml: "session_ttl_minutes: 60\n", want: 60},
		{name: "minimum", yaml: "session_ttl_minutes: 1\n", want: 1},
		{name: "maximum", yaml: "session_ttl_minutes: 43200\n", want: 43200},
		{name: "zero", yaml: "session_ttl_minutes: 0\n", wantErr: true},
		{name: "negative", yaml: "session_ttl_minutes: -1\n", wantErr: true},
		{name: "too large", yaml: "session_ttl_minutes: 43201\n", wantErr: true},
		{name: "not an integer", yaml: "session_ttl_minutes: 15m\n", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := parseConfig([]byte(test.yaml))
			if test.wantErr {
				if err == nil {
					t.Fatalf("accepted invalid session TTL: %d", config.SessionTTLMinutes)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if config.SessionTTLMinutes != test.want {
				t.Fatalf("session TTL = %d, want %d", config.SessionTTLMinutes, test.want)
			}
		})
	}
}

func TestResponseCompressionConfig(t *testing.T) {
	tests := []struct {
		name          string
		yaml          string
		wantEnabled   bool
		wantThreshold int
		wantErr       bool
	}{
		{name: "defaults", wantEnabled: true, wantThreshold: defaultResponseCompressionMinBytes},
		{name: "explicitly disabled", yaml: "response_compression: false\n", wantThreshold: defaultResponseCompressionMinBytes},
		{name: "zero threshold", yaml: "response_compression_min_bytes: 0\n", wantEnabled: true},
		{name: "custom threshold", yaml: "response_compression_min_bytes: 4096\n", wantEnabled: true, wantThreshold: 4096},
		{name: "negative threshold", yaml: "response_compression_min_bytes: -1\n", wantErr: true},
		{name: "threshold too large", yaml: "response_compression_min_bytes: 16777217\n", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := parseConfig([]byte(test.yaml))
			if test.wantErr {
				if err == nil {
					t.Fatalf("accepted response compression threshold %d", config.ResponseCompressionMinBytes)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if config.ResponseCompression != test.wantEnabled || config.ResponseCompressionMinBytes != test.wantThreshold {
				t.Fatalf("compression config = enabled %t, threshold %d", config.ResponseCompression, config.ResponseCompressionMinBytes)
			}
		})
	}
}

func TestPeakPricingConfig(t *testing.T) {
	config, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !config.PeakPricing {
		t.Fatal("the bundled peak schedule must be enabled by default")
	}
	disabled, err := parseConfig([]byte("peak_pricing: false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if disabled.PeakPricing {
		t.Fatal("peak_pricing: false was ignored")
	}
}

func TestTTFTSupplementConfig(t *testing.T) {
	config, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !config.TTFTSupplement || config.TTFTSupplementDir != "" {
		t.Fatalf("linkage defaults = %+v", config)
	}
	disabled, err := parseConfig([]byte("ttft_supplement: false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if disabled.TTFTSupplement {
		t.Fatal("ttft_supplement: false was ignored")
	}
	relative, err := parseConfig([]byte("ttft_supplement_dir: sibling-state\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(relative.TTFTSupplementDir) || !strings.HasSuffix(relative.TTFTSupplementDir, "sibling-state") {
		t.Fatalf("relative linkage dir was not normalized: %q", relative.TTFTSupplementDir)
	}
	// The default directory sits beside the database file.
	resolved := Config{DataPath: "/var/lib/cpa/data/usage.db"}.resolvedTTFTSupplementDir()
	if resolved != filepath.Join("/var/lib/cpa/data", defaultSiblingStateDirName) {
		t.Fatalf("default sibling dir = %q", resolved)
	}
}
