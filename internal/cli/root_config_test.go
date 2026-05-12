package cli

import "testing"

func TestConfigAPIURLForModeIgnoresLoopbackOutsideDev(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		devMode bool
		want    string
	}{
		{
			name: "prod config in normal mode",
			raw:  " https://flows.breyta.ai/ ",
			want: "https://flows.breyta.ai/",
		},
		{
			name: "custom non-loopback config in normal mode",
			raw:  "https://staging.example.test",
			want: "https://staging.example.test",
		},
		{
			name: "localhost config ignored in normal mode",
			raw:  "http://localhost:8090",
			want: "",
		},
		{
			name: "loopback ip config ignored in normal mode",
			raw:  "http://127.0.0.1:8090",
			want: "",
		},
		{
			name:    "localhost config allowed in dev mode",
			raw:     "http://localhost:8090",
			devMode: true,
			want:    "http://localhost:8090",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := configAPIURLForMode(tt.raw, tt.devMode); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
