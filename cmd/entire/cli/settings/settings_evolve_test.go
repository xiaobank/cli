package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetEvolveConfig_NilConfig_ReturnsDefaults(t *testing.T) {
	t.Parallel()

	s := &EntireSettings{}
	cfg := s.GetEvolveConfig()

	assert.False(t, cfg.Enabled)
	assert.Equal(t, 5, cfg.SessionThreshold)
}

func TestGetEvolveConfig_ZeroThreshold_DefaultsFiveToFive(t *testing.T) {
	t.Parallel()

	s := &EntireSettings{
		EvolveConfig: &EvolveSettings{
			Enabled:          true,
			SessionThreshold: 0,
		},
	}
	cfg := s.GetEvolveConfig()

	assert.True(t, cfg.Enabled)
	assert.Equal(t, 5, cfg.SessionThreshold)
}

func TestGetEvolveConfig_ExplicitValues_Preserved(t *testing.T) {
	t.Parallel()

	s := &EntireSettings{
		EvolveConfig: &EvolveSettings{
			Enabled:          true,
			SessionThreshold: 10,
		},
	}
	cfg := s.GetEvolveConfig()

	assert.True(t, cfg.Enabled)
	assert.Equal(t, 10, cfg.SessionThreshold)
}

func TestGetEvolveConfig_ExplicitDisabled_Preserved(t *testing.T) {
	t.Parallel()

	s := &EntireSettings{
		EvolveConfig: &EvolveSettings{
			Enabled:          false,
			SessionThreshold: 3,
		},
	}
	cfg := s.GetEvolveConfig()

	assert.False(t, cfg.Enabled)
	assert.Equal(t, 3, cfg.SessionThreshold)
}
