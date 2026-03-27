package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSkillCmd_Registers(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()

	var found bool
	for _, cmd := range root.Commands() {
		if cmd.Name() == "skill" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected 'skill' command to be registered")
}

func TestSkillCmd_HasExpectedMetadata(t *testing.T) {
	t.Parallel()

	cmd := newSkillCmd()
	assert.Equal(t, "skill", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotNil(t, cmd.RunE)
}
