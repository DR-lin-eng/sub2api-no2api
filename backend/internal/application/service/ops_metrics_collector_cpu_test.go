package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCgroupCPUPercent(t *testing.T) {
	tests := []struct {
		name    string
		usage   float64
		elapsed float64
		cores   float64
		want    float64
	}{
		{name: "one full core of four", usage: 1, elapsed: 1, cores: 4, want: 25},
		{name: "quota fraction", usage: 0.5, elapsed: 1, cores: 0.5, want: 100},
		{name: "clamped", usage: 3, elapsed: 1, cores: 2, want: 100},
		{name: "zero elapsed", usage: 1, elapsed: 0, cores: 2, want: 0},
		{name: "zero cores", usage: 1, elapsed: 1, cores: 0, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeCgroupCPUPercent(tt.usage, tt.elapsed, tt.cores))
		})
	}
}
