package config

import "time"

// TimeoutConfig contains all timeout configurations for the system
type TimeoutConfig struct {
	// Consensus timeouts
	ProposeTimeout     time.Duration `mapstructure:"propose_timeout"`
	CollectTimeout     time.Duration `mapstructure:"collect_timeout"`
	FinalizeTimeout    time.Duration `mapstructure:"finalize_timeout"`
	CheckpointInterval time.Duration `mapstructure:"checkpoint_interval"`

	// Authentication and caching
	AuthCacheTTL     time.Duration `mapstructure:"auth_cache_ttl"`
	AuthCacheCleanup time.Duration `mapstructure:"auth_cache_cleanup"`
	AuthMaxTimeDrift time.Duration `mapstructure:"auth_max_time_drift"`

	// Task management
	TaskResponseTimeout    time.Duration `mapstructure:"task_response_timeout"`
	TaskSendTimeout        time.Duration `mapstructure:"task_send_timeout"`
	TaskCleanupInterval    time.Duration `mapstructure:"task_cleanup_interval"`
	TaskReassignMinTime    time.Duration `mapstructure:"task_reassign_min_time"`
	RootLayerSubmitTimeout time.Duration `mapstructure:"rootlayer_submit_timeout"`

	// Network and retry
	NATSReconnectWait   time.Duration `mapstructure:"nats_reconnect_wait"`
	WindowCheckInterval time.Duration `mapstructure:"window_check_interval"`
	RetryInterval       time.Duration `mapstructure:"retry_interval"`

	// SDK specific
	SDKTaskTimeout       time.Duration `mapstructure:"sdk_task_timeout"`
	SDKStreamRetryInterval time.Duration `mapstructure:"sdk_stream_retry_interval"`
	SDKSubmitBidTimeout   time.Duration `mapstructure:"sdk_submit_bid_timeout"`
	SDKClientTimeout      time.Duration `mapstructure:"sdk_client_timeout"`

	// Health and monitoring
	HealthCheckInterval time.Duration `mapstructure:"health_check_interval"`
	MetricsInterval     time.Duration `mapstructure:"metrics_interval"`

	// Mock and testing
	IntentGenerateInterval time.Duration `mapstructure:"intent_generate_interval"`
	StatusReportInterval   time.Duration `mapstructure:"status_report_interval"`

	// Report validation
	MaxExecutionTime time.Duration `mapstructure:"max_execution_time"`
	MaxTimeDrift     time.Duration `mapstructure:"max_time_drift"`
}

// DefaultTimeoutConfig returns default timeout configurations
func DefaultTimeoutConfig() *TimeoutConfig {
	return &TimeoutConfig{
		// Consensus
		ProposeTimeout:     5 * time.Second,
		CollectTimeout:     10 * time.Second,
		FinalizeTimeout:    2 * time.Second,
		CheckpointInterval: 30 * time.Second,

		// Authentication
		AuthCacheTTL:     5 * time.Minute,
		AuthCacheCleanup: 10 * time.Minute,
		AuthMaxTimeDrift: 5 * time.Minute,

		// Task management
		TaskResponseTimeout:    30 * time.Second,
		TaskSendTimeout:        5 * time.Second,
		TaskCleanupInterval:    1 * time.Hour,
		TaskReassignMinTime:    30 * time.Second,
		RootLayerSubmitTimeout: 5 * time.Second,

		// Network
		NATSReconnectWait:   2 * time.Second,
		WindowCheckInterval: 1 * time.Second,
		RetryInterval:       5 * time.Second,

		// SDK
		SDKTaskTimeout:         30 * time.Second,
		SDKStreamRetryInterval: 5 * time.Second,
		SDKSubmitBidTimeout:    5 * time.Second,
		SDKClientTimeout:       5 * time.Second,

		// Health
		HealthCheckInterval: 30 * time.Second,
		MetricsInterval:     30 * time.Second,

		// Mock
		IntentGenerateInterval: 15 * time.Second,
		StatusReportInterval:   30 * time.Second,

		// Report validation
		MaxExecutionTime: 30 * time.Minute,
		MaxTimeDrift:     5 * time.Minute,
	}
}

// NetworkConfig contains network and address configurations
type NetworkConfig struct {
	// NATS configuration
	NATSUrl string `mapstructure:"nats_url"`

	// Default ports
	ValidatorGRPCPort string `mapstructure:"validator_grpc_port"`
	MatcherGRPCPort   string `mapstructure:"matcher_grpc_port"`
	MetricsPort       string `mapstructure:"metrics_port"`

	// Default addresses
	MatcherDefaultAddr   string `mapstructure:"matcher_default_addr"`
	ValidatorDefaultAddr string `mapstructure:"validator_default_addr"`
	RootLayerDefaultAddr string `mapstructure:"rootlayer_default_addr"`
}

// DefaultNetworkConfig returns default network configurations
func DefaultNetworkConfig() *NetworkConfig {
	return &NetworkConfig{
		NATSUrl:              "nats://127.0.0.1:4222",
		ValidatorGRPCPort:    ":9090",
		MatcherGRPCPort:      ":8090",
		MetricsPort:          ":9095",
		MatcherDefaultAddr:   "localhost:8090",
		ValidatorDefaultAddr: "localhost:9090",
		RootLayerDefaultAddr: "localhost:9090",
	}
}

// IdentityConfig contains identity configurations
type IdentityConfig struct {
	SubnetID    string `mapstructure:"subnet_id"`
	ValidatorID string `mapstructure:"validator_id"`
	MatcherID   string `mapstructure:"matcher_id"`
	AgentID     string `mapstructure:"agent_id"`
}

// DefaultIdentityConfig returns default identity configurations
// IMPORTANT: IDs must be explicitly configured - no defaults provided
func DefaultIdentityConfig() *IdentityConfig {
	return &IdentityConfig{
		// All IDs must be explicitly configured
		// Using default IDs would cause conflicts and security issues
		SubnetID:    "", // MUST be configured
		ValidatorID: "", // MUST be configured
		MatcherID:   "", // MUST be configured
		AgentID:     "", // MUST be configured
	}
}

// LimitsConfig contains various system limits
type LimitsConfig struct {
	MaxBidsPerIntent    int `mapstructure:"max_bids_per_intent"`
	BiddingWindowSec    int `mapstructure:"bidding_window_sec"`
	MaxRetries          int `mapstructure:"max_retries"`
	MaxConcurrentTasks  int `mapstructure:"max_concurrent_tasks"`
	DuplicateCheckWindow time.Duration `mapstructure:"duplicate_check_window"`
}

// DefaultLimitsConfig returns default limits configurations
func DefaultLimitsConfig() *LimitsConfig {
	return &LimitsConfig{
		MaxBidsPerIntent:     100,
		BiddingWindowSec:     10,
		MaxRetries:           3,
		MaxConcurrentTasks:   100,
		DuplicateCheckWindow: 1 * time.Second,
	}
}