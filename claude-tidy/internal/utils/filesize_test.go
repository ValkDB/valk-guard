package utils

import "testing"

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name string
		b    int64
		want string
	}{
		{"zero bytes", 0, "0 B"},
		{"small bytes", 512, "512 B"},
		{"one KB", 1024, "1.0 KB"},
		{"several KB", 15360, "15.0 KB"},
		{"one MB", 1048576, "1.0 MB"},
		{"several MB", 52428800, "50.0 MB"},
		{"one GB", 1073741824, "1.0 GB"},
		{"several GB", 5368709120, "5.0 GB"},
		{"fractional GB", 1610612736, "1.5 GB"},
		{"fractional MB", 1572864, "1.5 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatBytes(tt.b)
			if got != tt.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.b, got, tt.want)
			}
		})
	}
}
