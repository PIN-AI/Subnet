package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigurationLoading(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configContent := `
subnet_id: "test-subnet"

identity:
  subnet_id: "configured-subnet"
  validator_id: "configured-validator"
  matcher_id: "configured-matcher"
  agent_id: "configured-agent"

timeouts:
  propose_timeout: 5s
  collect_timeout: 10s
  task_send_timeout: 3s
  task_cleanup_interval: 30m
  rootlayer_submit_timeout: 7s

network:
  validator_grpc_port: ":9999"

limits:
  max_bids_per_intent: 50
  bidding_window_sec: 15

validator_set:
  min_validators: 3
  threshold_num: 2
  threshold_denom: 3
  validators:
    - id: "v1"
      endpoint: "127.0.0.1:7001"
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// Load the config
	cfg, err := Load(configFile)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Test Identity configuration
	assert.NotNil(t, cfg.Identity)
	assert.Equal(t, "configured-subnet", cfg.Identity.SubnetID)
	assert.Equal(t, "configured-validator", cfg.Identity.ValidatorID)
	assert.Equal(t, "configured-matcher", cfg.Identity.MatcherID)
	assert.Equal(t, "configured-agent", cfg.Identity.AgentID)

	// Test Timeouts configuration
	assert.NotNil(t, cfg.Timeouts)
	assert.Equal(t, 5*time.Second, cfg.Timeouts.ProposeTimeout)
	assert.Equal(t, 10*time.Second, cfg.Timeouts.CollectTimeout)
	assert.Equal(t, 3*time.Second, cfg.Timeouts.TaskSendTimeout)
	assert.Equal(t, 30*time.Minute, cfg.Timeouts.TaskCleanupInterval)
	assert.Equal(t, 7*time.Second, cfg.Timeouts.RootLayerSubmitTimeout)

	// Test Network configuration
	assert.NotNil(t, cfg.Network)
	assert.Equal(t, ":9999", cfg.Network.ValidatorGRPCPort)

	// Test Limits configuration
	assert.NotNil(t, cfg.Limits)
	assert.Equal(t, 50, cfg.Limits.MaxBidsPerIntent)
	assert.Equal(t, 15, cfg.Limits.BiddingWindowSec)

	// Test ValidatorSet configuration
	assert.NotNil(t, cfg.ValidatorSet)
	assert.Equal(t, 3, cfg.ValidatorSet.MinValidators)
	assert.Equal(t, 2, cfg.ValidatorSet.ThresholdNum)
	assert.Equal(t, 3, cfg.ValidatorSet.ThresholdDenom)
}

func TestDefaultTimeoutConfig(t *testing.T) {
	cfg := DefaultTimeoutConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, 5*time.Second, cfg.ProposeTimeout)
	assert.Equal(t, 10*time.Second, cfg.CollectTimeout)
	assert.Equal(t, 5*time.Second, cfg.TaskSendTimeout)
	assert.Equal(t, time.Hour, cfg.TaskCleanupInterval)
	assert.Equal(t, 5*time.Second, cfg.RootLayerSubmitTimeout)
}

func TestDefaultIdentityConfig(t *testing.T) {
	cfg := DefaultIdentityConfig()

	assert.NotNil(t, cfg)
	// Default IDs should be empty to force configuration
	assert.Equal(t, "", cfg.SubnetID, "SubnetID should be empty by default")
	assert.Equal(t, "", cfg.ValidatorID, "ValidatorID should be empty by default")
	assert.Equal(t, "", cfg.MatcherID, "MatcherID should be empty by default")
	assert.Equal(t, "", cfg.AgentID, "AgentID should be empty by default")
}