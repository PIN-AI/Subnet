package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
	"subnet/internal/types"
)

type ValidatorConfig struct {
	ID       string `mapstructure:"id"`
	Endpoint string `mapstructure:"endpoint"`
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
}

type RootLayerConfig struct {
	HTTPURL            string `mapstructure:"http_url"`
	WSURL              string `mapstructure:"ws_url"`
	AuthToken          string `mapstructure:"auth_token"`
	GRPCAddr           string `mapstructure:"grpc_addr"`
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
