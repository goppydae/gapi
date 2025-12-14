package agentmgr

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/goppydae/gapi/internal/cgroups"
)

func TestParseLimits(t *testing.T) {
	tests := []struct {
		name     string
		cpu      string
		mem      string
		expected cgroups.ResourceSpec
	}{
		{
			name:     "Empty",
			cpu:      "",
			mem:      "",
			expected: cgroups.ResourceSpec{},
		},
		{
			name: "CPU Decimal",
			cpu:  "0.5",
			mem:  "",
			expected: cgroups.ResourceSpec{
				CPU: 0.5,
			},
		},
		{
			name: "CPU Milli",
			cpu:  "500m",
			mem:  "",
			expected: cgroups.ResourceSpec{
				CPU: 0.5,
			},
		},
		{
			name: "CPU Integer",
			cpu:  "2",
			mem:  "",
			expected: cgroups.ResourceSpec{
				CPU: 2.0,
			},
		},
		{
			name: "Mem Bytes",
			cpu:  "",
			mem:  "1024",
			expected: cgroups.ResourceSpec{
				Memory: 1024,
			},
		},
		{
			name: "Mem KB",
			cpu:  "",
			mem:  "10KB",
			expected: cgroups.ResourceSpec{
				Memory: 10 * 1024,
			},
		},
		{
			name: "Mem MB",
			cpu:  "",
			mem:  "10MB",
			expected: cgroups.ResourceSpec{
				Memory: 10 * 1024 * 1024,
			},
		},
		{
			name: "Mem GB",
			cpu:  "",
			mem:  "1GB",
			expected: cgroups.ResourceSpec{
				Memory: 1 * 1024 * 1024 * 1024,
			},
		},
		{
			name: "Mixed",
			cpu:  "100m",
			mem:  "512M",
			expected: cgroups.ResourceSpec{
				CPU:    0.1,
				Memory: 512 * 1024 * 1024,
			},
		},
		{
			name: "Invalid CPU",
			cpu:  "foo",
			mem:  "10MB",
			expected: cgroups.ResourceSpec{
				Memory: 10 * 1024 * 1024,
			},
		},
		{
			name: "Invalid Mem",
			cpu:  "0.1",
			mem:  "bar",
			expected: cgroups.ResourceSpec{
				CPU: 0.1,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseLimits(tc.cpu, tc.mem)
			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("parseLimits mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
