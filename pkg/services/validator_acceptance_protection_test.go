package services

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPolicyValidator_AcceptanceTestProtection(t *testing.T) {
	tmpDir := t.TempDir()
	v := NewPolicyValidator([]string{"make"}, "main", nil)
	state := &domain.State{ProjectPath: tmpDir}

	workerRoles := []string{"generator", "tester"}
	privilegedRoles := []string{"qa", "orchestrator"}

	mutationTools := []string{"write_file", "edit_file", "delete_file", "apply_patch"}

	for _, role := range workerRoles {
		ctx := context.WithValue(context.Background(), AgentRoleKey, role)

		for _, tool := range mutationTools {
			t.Run("worker role "+role+" blocked from "+tool+" on tests/acceptance", func(t *testing.T) {
				args := map[string]any{
					"path":           "tests/acceptance/test_contract.py",
					"content":        "assert True",
					"target_content": "old",
					"new_content":    "new",
					"patch":          "diff",
				}
				res, err := v.Validate(ctx, domain.Action{Tool: tool, Args: args}, state)
				require.NoError(t, err)
				assert.False(t, res.Allowed, "expected %s with tool %s on acceptance tests to be blocked", role, tool)
				assert.Contains(t, res.Reason, "independent QA acceptance tests in 'tests/acceptance/' are immutable")
			})
		}

		t.Run("worker role "+role+" blocked from write_files on tests/acceptance", func(t *testing.T) {
			args := map[string]any{
				"files": map[string]any{
					"tests/acceptance/test_suite.sh": "#!/bin/sh\nexit 0\n",
				},
			}
			res, err := v.Validate(ctx, domain.Action{Tool: "write_files", Args: args}, state)
			require.NoError(t, err)
			assert.False(t, res.Allowed)
			assert.Contains(t, res.Reason, "independent QA acceptance tests in 'tests/acceptance/' are immutable")
		})

		t.Run("worker role "+role+" allowed to write in tests/unit", func(t *testing.T) {
			args := map[string]any{
				"path":    "tests/unit/test_module.py",
				"content": "def test_ok(): pass",
			}
			res, err := v.Validate(ctx, domain.Action{Tool: "write_file", Args: args}, state)
			require.NoError(t, err)
			assert.True(t, res.Allowed)
		})
	}

	for _, role := range privilegedRoles {
		ctx := context.WithValue(context.Background(), AgentRoleKey, role)
		t.Run("privileged role "+role+" allowed to write in tests/acceptance", func(t *testing.T) {
			args := map[string]any{
				"path":    "tests/acceptance/test_contract.py",
				"content": "def test_acceptance(): pass",
			}
			res, err := v.Validate(ctx, domain.Action{Tool: "write_file", Args: args}, state)
			require.NoError(t, err)
			assert.True(t, res.Allowed, "privileged role %s must be allowed to write acceptance tests", role)
		})
	}
}
