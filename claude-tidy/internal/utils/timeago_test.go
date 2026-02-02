package utils

import (
	"testing"
	"time"
)

func TestTimeAgo(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"just now", now.Add(-10 * time.Second), "just now"},
		{"1 min ago", now.Add(-1 * time.Minute), "1 min ago"},
		{"30 mins ago", now.Add(-30 * time.Minute), "30m ago"},
		{"1 hour ago", now.Add(-1 * time.Hour), "1h ago"},
		{"5 hours ago", now.Add(-5 * time.Hour), "5h ago"},
		{"1 day ago", now.Add(-24 * time.Hour), "1d ago"},
		{"3 days ago", now.Add(-3 * 24 * time.Hour), "3d ago"},
		{"1 week ago", now.Add(-7 * 24 * time.Hour), "1w ago"},
		{"2 weeks ago", now.Add(-14 * 24 * time.Hour), "2w ago"},
		{"1 month ago", now.Add(-35 * 24 * time.Hour), "1mo ago"},
		{"3 months ago", now.Add(-90 * 24 * time.Hour), "3mo ago"},
		{"1 year ago", now.Add(-400 * 24 * time.Hour), "1y ago"},
		{"2 years ago", now.Add(-800 * 24 * time.Hour), "2y ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TimeAgo(tt.t)
			if got != tt.want {
				t.Errorf("TimeAgo(%v) = %q, want %q", tt.t, got, tt.want)
			}
		})
	}
}
