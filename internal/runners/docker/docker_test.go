package docker

import (
	"testing"

	"leil.io/leil-tests/internal/utils"
)

func TestGetDefaultHostConfigSetsNanoCPUsInDockerUnits(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		cpuLimit    int
		wantNanoCPU int64
	}{
		{
			name:        "zero CPUs (unlimited)",
			cpuLimit:    0,
			wantNanoCPU: 0,
		},
		{
			name:        "negative CPUs (clamped to 0)",
			cpuLimit:    -1,
			wantNanoCPU: 0,
		},
		{
			name:        "one CPU",
			cpuLimit:    1,
			wantNanoCPU: nanoCPUsPerCPU,
		},
		{
			name:        "four CPUs",
			cpuLimit:    4,
			wantNanoCPU: 4 * nanoCPUsPerCPU,
		},
		{
			name:        "above max (clamped to avoid int64 overflow)",
			cpuLimit:    maxCPUCount + 1,
			wantNanoCPU: maxCPUCount * nanoCPUsPerCPU,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			config := getDefaultHostConfig(utils.TestOptions{CpuLimit: testCase.cpuLimit})

			if config.Resources.NanoCPUs != testCase.wantNanoCPU {
				t.Fatalf("NanoCPUs = %d, want %d", config.Resources.NanoCPUs,
					testCase.wantNanoCPU)
			}
		})
	}
}
