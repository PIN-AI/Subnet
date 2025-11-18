package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
	"subnet/internal/types"
)

type ValidatorConfig struct {
	ID            string `mapstructure:"id"`
	Endpoint      string `mapstructure:"endpoint"`
	Pubkey        string `mapstructure:"pubkey"`
	GRPCEndpoint  string `mapstructure:"grpc_endpoint"`
	RaftAddress   string `mapstructure:"raft_address"`
	GossipAddress string `mapstructure:"gossip_address"`
}

type ValidatorSetConfig struct {
	MinValidators  int               `mapstructure:"min_validators"`
	ThresholdNum   int               `mapstructure:"threshold_num"`
	ThresholdDenom int               `mapstructure:"threshold_denom"`
	Validators     []ValidatorConfig `mapstructure:"validators"`
}

func (cfg ValidatorSetConfig) ToValidatorSet() (*types.ValidatorSet, error) {
	vs := &types.ValidatorSet{
		MinValidators:  cfg.MinValidators,
		ThresholdNum:   cfg.ThresholdNum,
		ThresholdDenom: cfg.ThresholdDenom,
	}
	for _, v := range cfg.Validators {
		vs.Validators = append(vs.Validators, types.Validator{ID: v.ID, Endpoint: v.Endpoint})
	}
	if err := vs.Validate(); err != nil {
		return nil, err
	}
	vs.SortValidators()
	return vs, nil
}

type ConsensusConfig struct {
	ProposeTimeoutMs  int64 `mapstructure:"propose_timeout_ms"`
	CollectTimeoutMs  int64 `mapstructure:"collect_timeout_ms"`
	ChallengeWindowMs int64 `mapstructure:"challenge_window_ms"`
}

type StorageConfig struct {
	LevelDBPath string `mapstructure:"leveldb_path"`
}

type RPCConfig struct {
	ListenAddr string        `mapstructure:"listen_addr"`
	Auth       RPCAuthConfig `mapstructure:"auth"`
}

type MetricsConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	ListenAddr string `mapstructure:"listen_addr"` // e.g., 0.0.0.0:9090
}

type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}

type NATSConfig struct {
	URL string `mapstructure:"url"`
	// Optional custom subjects; defaults provided if empty
	SubjectPropose   string `mapstructure:"subject_propose"`
	SubjectSignature string `mapstructure:"subject_signature"`
	SubjectFinalized string `mapstructure:"subject_finalized"`
	DedupeTTLSeconds int    `mapstructure:"dedupe_ttl_seconds"`
	DedupeMax        int    `mapstructure:"dedupe_max"`
}

type RecoveryConfig struct {
	// mode: prune_to_continuous | align_to_anchor | rebuild_from_anchor
	Mode string `mapstructure:"mode"`
}

// RaftPeerConfig represents a single Raft peer
type RaftPeerConfig struct {
	ID      string `mapstructure:"id"`
	Address string `mapstructure:"address"`
}

// RaftConfig represents Raft consensus configuration
type RaftConfig struct {
	Enable    bool             `mapstructure:"enable"`
	Bootstrap bool             `mapstructure:"bootstrap"`
	Bind      string           `mapstructure:"bind"`
	Advertise string           `mapstructure:"advertise"`
	DataDir   string           `mapstructure:"data_dir"`
	Peers     []RaftPeerConfig `mapstructure:"peers"`
}

// GossipConfig represents Gossip protocol configuration
type GossipConfig struct {
	Enable         bool     `mapstructure:"enable"`
	BindAddress    string   `mapstructure:"bind_address"`
	BindPort       int      `mapstructure:"bind_port"`
	Seeds          []string `mapstructure:"seeds"`
	GossipInterval string   `mapstructure:"gossip_interval"`
	ProbeInterval  string   `mapstructure:"probe_interval"`
}

type DemoSigner struct {
	ID         string `mapstructure:"id"`
	PrivKeyHex string `mapstructure:"privkey_hex"`
}

type AgentExecutorConfig struct {
	Type   string                 `mapstructure:"type"`
	Config map[string]interface{} `mapstructure:"config"`
}

type AgentSignerConfig struct {
	Type       string                 `mapstructure:"type"`
	PrivateKey string                 `mapstructure:"private_key"`
	Config     map[string]interface{} `mapstructure:"config"`
}

type AgentMatcherConfig struct {
	Strategy string                 `mapstructure:"strategy"`
	Config   map[string]interface{} `mapstructure:"config"`
	Signer   AgentSignerConfig      `mapstructure:"signer"`
}

type AgentConfig struct {
	Executor AgentExecutorConfig `mapstructure:"executor"`
	Matcher  AgentMatcherConfig  `mapstructure:"matcher"`
}

type AppConfig struct {
	SubnetID     string                 `mapstructure:"subnet_id"`
	Identity     IdentityConfig         `mapstructure:"identity"`
	Timeouts     TimeoutConfig          `mapstructure:"timeouts"`
	Network      NetworkConfig          `mapstructure:"network"`
	Limits       LimitsConfig           `mapstructure:"limits"`
	ValidatorSet ValidatorSetConfig     `mapstructure:"validator_set"`
	Consensus    ConsensusConfig        `mapstructure:"consensus"`
	Storage      StorageConfig          `mapstructure:"storage"`
	RPC          RPCConfig              `mapstructure:"rpc"`
	Metrics      MetricsConfig          `mapstructure:"metrics"`
	TLS          TLSConfig              `mapstructure:"tls"`
	NATS         NATSConfig             `mapstructure:"nats"`
	Recovery     RecoveryConfig         `mapstructure:"recovery"`
	DemoSigners  []DemoSigner           `mapstructure:"demo_signers"`
	Agent        AgentConfig            `mapstructure:"agent"`
	Validator    map[string]interface{} `mapstructure:"validator"`
	RootLayer    RootLayerConfig        `mapstructure:"rootlayer"`
	Blockchain   *BlockchainConfig      `mapstructure:"blockchain"`
	Raft         RaftConfig             `mapstructure:"raft"`
	Gossip       GossipConfig           `mapstructure:"gossip"`
}

type RootLayerConfig struct {
	HTTPURL            string `mapstructure:"http_url"`
	WSURL              string `mapstructure:"ws_url"`
	AuthToken          string `mapstructure:"auth_token"`
	GRPCAddr           string `mapstructure:"grpc_addr"`
	GRPCEndpoint       string `mapstructure:"grpc_endpoint"` // Alternative field name
	GRPCTLS            bool   `mapstructure:"grpc_tls"`
	GRPCDialTimeoutRaw string `mapstructure:"grpc_dial_timeout"`
	ConnectTimeoutRaw  string `mapstructure:"connect_timeout"`
	RequestTimeoutRaw  string `mapstructure:"request_timeout"`
	RetryDelayRaw      string `mapstructure:"retry_delay"`
	MaxRetries         int    `mapstructure:"max_retries"`
	ConnectTimeout     time.Duration
	RequestTimeout     time.Duration
	RetryDelay         time.Duration
	GRPCDialTimeout    time.Duration
}

type RPCAuthConfig struct {
	Enabled bool `mapstructure:"enabled"`
	// Static bearer tokens for agent services (MVP). In production, replace with mTLS or OIDC/JWT.
	BearerTokens []string `mapstructure:"bearer_tokens"`
}

func (c *AgentConfig) normalize() {
	if c.Executor.Config == nil {
		c.Executor.Config = map[string]interface{}{}
	}
	if c.Matcher.Config == nil {
		c.Matcher.Config = map[string]interface{}{}
	}
	if c.Matcher.Signer.Config == nil {
		c.Matcher.Signer.Config = map[string]interface{}{}
	}
}

func (c *RootLayerConfig) normalize() error {
	parseDuration := func(raw string, fallback time.Duration) (time.Duration, error) {
		if raw == "" {
			return fallback, nil
		}
		d, err := time.ParseDuration(raw)
		if err != nil {
			return 0, err
		}
		return d, nil
	}

	var err error
	c.ConnectTimeout, err = parseDuration(c.ConnectTimeoutRaw, 10*time.Second)
	if err != nil {
		return fmt.Errorf("invalid rootlayer.connect_timeout: %w", err)
	}
	c.RequestTimeout, err = parseDuration(c.RequestTimeoutRaw, 30*time.Second)
	if err != nil {
		return fmt.Errorf("invalid rootlayer.request_timeout: %w", err)
	}
	c.RetryDelay, err = parseDuration(c.RetryDelayRaw, 1*time.Second)
	if err != nil {
		return fmt.Errorf("invalid rootlayer.retry_delay: %w", err)
	}
	c.GRPCDialTimeout, err = parseDuration(c.GRPCDialTimeoutRaw, 5*time.Second)
	if err != nil {
		return fmt.Errorf("invalid rootlayer.grpc_dial_timeout: %w", err)
	}
	if c.MaxRetries < 0 {
		c.MaxRetries = 0
	}
	return nil
}

func Load(path string) (*AppConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.AutomaticEnv()
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	var cfg AppConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	cfg.Agent.normalize()
	if err := cfg.RootLayer.normalize(); err != nil {
		return nil, err
	}
	if cfg.Blockchain == nil {
		cfg.Blockchain = DefaultBlockchainConfig()
	}
	if err := cfg.Blockchain.Normalize(); err != nil {
		return nil, err
	}

	// Validate configuration
	validator := NewConfigValidator()
	if err := validator.Validate(&cfg); err != nil {
		return nil, err
	}

	// Print configuration summary
	PrintConfigurationSummary(&cfg)

	return &cfg, nil
}

func (c ConsensusConfig) ProposeTimeout() time.Duration {
	return time.Duration(c.ProposeTimeoutMs) * time.Millisecond
}
func (c ConsensusConfig) CollectTimeout() time.Duration {
	return time.Duration(c.CollectTimeoutMs) * time.Millisecond
}
func (c ConsensusConfig) ChallengeWindow() time.Duration {
	return time.Duration(c.ChallengeWindowMs) * time.Millisecond
}

// LoadSplit loads configuration from separate subnet.yaml and validator-specific.yaml files.
// It merges the two configurations with validator-specific settings taking precedence.
// It also auto-populates Raft peers and Gossip seeds from the validator_set.
func LoadSplit(subnetConfigPath, validatorConfigPath string) (*AppConfig, error) {
	// Load subnet configuration (shared)
	subnetViper := viper.New()
	subnetViper.SetConfigFile(subnetConfigPath)
	subnetViper.SetConfigType("yaml")
	if err := subnetViper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read subnet config: %w", err)
	}

	var subnetCfg AppConfig
	if err := subnetViper.Unmarshal(&subnetCfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal subnet config: %w", err)
	}

	// Load validator-specific configuration
	validatorViper := viper.New()
	validatorViper.SetConfigFile(validatorConfigPath)
	validatorViper.SetConfigType("yaml")
	if err := validatorViper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read validator config: %w", err)
	}

	var validatorCfg AppConfig
	if err := validatorViper.Unmarshal(&validatorCfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal validator config: %w", err)
	}

	// Merge configurations (validator-specific overrides subnet)
	cfg := mergeConfigs(&subnetCfg, &validatorCfg)

	// Auto-populate Raft peers from validator_set
	if cfg.Raft.Enable && len(cfg.Raft.Peers) == 0 {
		cfg.Raft.Peers = make([]RaftPeerConfig, 0, len(cfg.ValidatorSet.Validators))
		for _, v := range cfg.ValidatorSet.Validators {
			// Extract raft_address from validator
			// In subnet.yaml, validators have raft_address field
			raftAddr := extractValidatorRaftAddress(v.ID, &subnetCfg)
			if raftAddr != "" {
				cfg.Raft.Peers = append(cfg.Raft.Peers, RaftPeerConfig{
					ID:      v.ID,
					Address: raftAddr,
				})
			}
		}
	}

	// Auto-populate Gossip seeds from validator_set
	if cfg.Gossip.Enable && len(cfg.Gossip.Seeds) == 0 {
		cfg.Gossip.Seeds = make([]string, 0, len(cfg.ValidatorSet.Validators))
		for _, v := range cfg.ValidatorSet.Validators {
			// Extract gossip_address from validator
			gossipAddr := extractValidatorGossipAddress(v.ID, &subnetCfg)
			if gossipAddr != "" {
				cfg.Gossip.Seeds = append(cfg.Gossip.Seeds, gossipAddr)
			}
		}
	}

	// Normalize and validate
	cfg.Agent.normalize()
	if err := cfg.RootLayer.normalize(); err != nil {
		return nil, err
	}
	if cfg.Blockchain == nil {
		cfg.Blockchain = DefaultBlockchainConfig()
	}
	if err := cfg.Blockchain.Normalize(); err != nil {
		return nil, err
	}

	// Validate configuration
	validator := NewConfigValidator()
	if err := validator.Validate(cfg); err != nil {
		return nil, err
	}

	// Print configuration summary
	PrintConfigurationSummary(cfg)

	return cfg, nil
}

// mergeConfigs merges subnet config with validator-specific config.
// Validator-specific settings take precedence over subnet settings.
func mergeConfigs(subnet, validator *AppConfig) *AppConfig {
	merged := *subnet // Start with subnet config

	// Override with validator-specific settings
	if validator.Identity.ValidatorID != "" {
		merged.Identity.ValidatorID = validator.Identity.ValidatorID
	}
	if validator.Network.ValidatorGRPCPort != "" {
		merged.Network.ValidatorGRPCPort = validator.Network.ValidatorGRPCPort
	}
	if validator.Network.MetricsPort != "" {
		merged.Network.MetricsPort = validator.Network.MetricsPort
	}
	if validator.Storage.LevelDBPath != "" {
		merged.Storage.LevelDBPath = validator.Storage.LevelDBPath
	}

	// Merge Raft config
	if validator.Raft.Enable {
		merged.Raft.Enable = validator.Raft.Enable
	}
	if validator.Raft.Bootstrap {
		merged.Raft.Bootstrap = validator.Raft.Bootstrap
	}
	if validator.Raft.Bind != "" {
		merged.Raft.Bind = validator.Raft.Bind
	}
	if validator.Raft.Advertise != "" {
		merged.Raft.Advertise = validator.Raft.Advertise
	}
	if validator.Raft.DataDir != "" {
		merged.Raft.DataDir = validator.Raft.DataDir
	}
	if len(validator.Raft.Peers) > 0 {
		merged.Raft.Peers = validator.Raft.Peers
	}

	// Merge Gossip config
	if validator.Gossip.Enable {
		merged.Gossip.Enable = validator.Gossip.Enable
	}
	if validator.Gossip.BindAddress != "" {
		merged.Gossip.BindAddress = validator.Gossip.BindAddress
	}
	if validator.Gossip.BindPort != 0 {
		merged.Gossip.BindPort = validator.Gossip.BindPort
	}
	if len(validator.Gossip.Seeds) > 0 {
		merged.Gossip.Seeds = validator.Gossip.Seeds
	}
	if validator.Gossip.GossipInterval != "" {
		merged.Gossip.GossipInterval = validator.Gossip.GossipInterval
	}
	if validator.Gossip.ProbeInterval != "" {
		merged.Gossip.ProbeInterval = validator.Gossip.ProbeInterval
	}

	// Merge Metrics config
	if validator.Metrics.Enabled {
		merged.Metrics.Enabled = validator.Metrics.Enabled
	}
	if validator.Metrics.ListenAddr != "" {
		merged.Metrics.ListenAddr = validator.Metrics.ListenAddr
	}

	return &merged
}

// extractValidatorRaftAddress extracts the raft_address from validator_set in subnet config.
// The subnet.yaml file should have validators with raft_address field.
func extractValidatorRaftAddress(validatorID string, cfg *AppConfig) string {
	for _, v := range cfg.ValidatorSet.Validators {
		if v.ID == validatorID {
			return v.RaftAddress
		}
	}
	return ""
}

// extractValidatorGossipAddress extracts the gossip_address from validator_set in subnet config.
func extractValidatorGossipAddress(validatorID string, cfg *AppConfig) string {
	for _, v := range cfg.ValidatorSet.Validators {
		if v.ID == validatorID {
			return v.GossipAddress
		}
	}
	return ""
}
