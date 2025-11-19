package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// ValidationMode determines the strictness of configuration validation
type ValidationMode string

const (
	ValidationModeProduction  ValidationMode = "production"
	ValidationModeDevelopment ValidationMode = "development"
	ValidationModeTest        ValidationMode = "test"
)

// ConfigValidator validates configuration for production readiness
type ConfigValidator struct {
	mode     ValidationMode
	errors   []string
	warnings []string
}

// NewConfigValidator creates a new configuration validator
func NewConfigValidator() *ConfigValidator {
	mode := ValidationModeDevelopment // Default to development

	// Check environment variable
	if envMode := os.Getenv("SUBNET_MODE"); envMode != "" {
		switch strings.ToLower(envMode) {
		case "production", "prod":
			mode = ValidationModeProduction
		case "test", "testing":
			mode = ValidationModeTest
		case "development", "dev":
			mode = ValidationModeDevelopment
		}
	}

	return &ConfigValidator{
		mode:     mode,
		errors:   []string{},
		warnings: []string{},
	}
}

// Validate checks the configuration for issues
func (v *ConfigValidator) Validate(cfg *AppConfig) error {
	v.errors = []string{}
	v.warnings = []string{}

	// Check identity configuration (CRITICAL)
	v.validateIdentity(cfg)

	// Check network configuration
	v.validateNetwork(cfg)

	// Check timeouts configuration
	v.validateTimeouts(cfg)

	// Check executor configuration
	v.validateExecutor(cfg)

	// Check signer configuration
	v.validateSigner(cfg)

	// Check validator policy
	v.validateValidatorPolicy(cfg)

	// Check security settings
	v.validateSecurity(cfg)

	// Check blockchain verification settings
	v.validateBlockchain(cfg)

	// Check demo signers
	v.validateDemoSigners(cfg)

	// Return errors if in production mode
	if v.mode == ValidationModeProduction && len(v.errors) > 0 {
		return fmt.Errorf("configuration validation failed:\n%s", strings.Join(v.errors, "\n"))
	}

	// Print warnings
	if len(v.warnings) > 0 {
		fmt.Printf("Configuration warnings:\n%s\n", strings.Join(v.warnings, "\n"))
	}

	return nil
}

func (v *ConfigValidator) validateExecutor(cfg *AppConfig) {
	if cfg == nil {
		return
	}

	execType := strings.TrimSpace(cfg.Agent.Executor.Type)
	if execType == "" {
		v.warnings = append(v.warnings, "No executor type configured")
		return
	}

	if execType == "dummy" {
		msg := "Using dummy executor - not suitable for production"
		if v.mode == ValidationModeProduction {
			v.errors = append(v.errors, msg)
		} else {
			v.warnings = append(v.warnings, msg)
		}
	}

	if execType == "script" {
		if raw, ok := cfg.Agent.Executor.Config["enable_sandbox"]; ok {
			if sandbox, okBool := raw.(bool); okBool && !sandbox {
				v.warnings = append(v.warnings, "Script executor sandbox is disabled - potential security risk")
			}
		} else {
			v.warnings = append(v.warnings, "Script executor sandbox configuration missing - defaulting to enabled")
		}
	}
}

func (v *ConfigValidator) validateSigner(cfg *AppConfig) {
	if cfg == nil {
		return
	}

	signerType := strings.TrimSpace(cfg.Agent.Matcher.Signer.Type)
	if signerType == "" {
		if v.mode == ValidationModeProduction {
			v.errors = append(v.errors, "No signer configured - assignments will not be signed")
		}
		return
	}

	if signerType == "noop" {
		msg := "Using noop signer - assignments will not be signed"
		if v.mode == ValidationModeProduction {
			v.errors = append(v.errors, msg)
		} else {
			v.warnings = append(v.warnings, msg)
		}
	}

	if signerType == "ecdsa" {
		privKey := strings.TrimSpace(cfg.Agent.Matcher.Signer.PrivateKey)
		if privKey == "" {
			v.errors = append(v.errors, "ECDSA signer configured but no private key provided")
		} else {
			if strings.HasPrefix(privKey, "0x01234567") {
				v.errors = append(v.errors, "Using example ECDSA private key - replace with secure key management")
			}
			if v.mode == ValidationModeProduction {
				v.warnings = append(v.warnings, "Private key in configuration file - use secure key management (HSM/KMS) in production")
			}
		}
	}
}

func (v *ConfigValidator) validateIdentity(cfg *AppConfig) {
	// Identity configuration is CRITICAL - IDs must be unique per node
	// Check subnet ID
	if cfg.Identity.SubnetID == "" {
		v.errors = append(v.errors, "CRITICAL: identity.subnet_id is required - must be unique per subnet")
	}

	// Check node-specific IDs based on what's being used
	// Note: Not all IDs are required for all node types
	// But if a node type needs an ID, it MUST be configured

	// Validate ID format (should not contain spaces or special chars)
	if cfg.Identity.SubnetID != "" && !isValidID(cfg.Identity.SubnetID) {
		v.errors = append(v.errors, fmt.Sprintf("invalid subnet_id format: %s", cfg.Identity.SubnetID))
	}
	if cfg.Identity.ValidatorID != "" && !isValidID(cfg.Identity.ValidatorID) {
		v.errors = append(v.errors, fmt.Sprintf("invalid validator_id format: %s", cfg.Identity.ValidatorID))
	}
	if cfg.Identity.MatcherID != "" && !isValidID(cfg.Identity.MatcherID) {
		v.errors = append(v.errors, fmt.Sprintf("invalid matcher_id format: %s", cfg.Identity.MatcherID))
	}
	if cfg.Identity.AgentID != "" && !isValidID(cfg.Identity.AgentID) {
		v.errors = append(v.errors, fmt.Sprintf("invalid agent_id format: %s", cfg.Identity.AgentID))
	}
}

// isValidID checks if an ID has valid format
func isValidID(id string) bool {
	// IDs should only contain alphanumeric, dash, underscore
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return len(id) > 0 && len(id) <= 64
}

func (v *ConfigValidator) validateNetwork(cfg *AppConfig) {
	// Network is embedded struct, not pointer

	// Validate ports
	v.validatePort("validator_grpc_port", cfg.Network.ValidatorGRPCPort)
	v.validatePort("matcher_grpc_port", cfg.Network.MatcherGRPCPort)
	v.validatePort("metrics_port", cfg.Network.MetricsPort)
}

func (v *ConfigValidator) validatePort(name string, port string) {
	if port == "" {
		return
	}
	// Port string like ":8080" or "0.0.0.0:8080"
	parts := strings.Split(port, ":")
	portStr := parts[len(parts)-1]
	if portNum, err := strconv.Atoi(portStr); err == nil {
		if portNum < 1024 {
			v.warnings = append(v.warnings, fmt.Sprintf("%s uses privileged port %d (< 1024)", name, portNum))
		}
		if portNum > 65535 {
			v.errors = append(v.errors, fmt.Sprintf("%s port out of range: %d", name, portNum))
		}
	}
}

func (v *ConfigValidator) validateTimeouts(cfg *AppConfig) {
	// Timeouts is embedded struct, not pointer

	// Critical timeouts should not be too short
	if cfg.Timeouts.ProposeTimeout > 0 && cfg.Timeouts.ProposeTimeout < time.Second {
		v.errors = append(v.errors, fmt.Sprintf("propose_timeout too short: %v", cfg.Timeouts.ProposeTimeout))
	}
	if cfg.Timeouts.CollectTimeout > 0 && cfg.Timeouts.CollectTimeout < 2*time.Second {
		v.errors = append(v.errors, fmt.Sprintf("collect_timeout too short: %v", cfg.Timeouts.CollectTimeout))
	}
	if cfg.Timeouts.TaskSendTimeout > 0 && cfg.Timeouts.TaskSendTimeout < 100*time.Millisecond {
		v.errors = append(v.errors, fmt.Sprintf("task_send_timeout too short: %v", cfg.Timeouts.TaskSendTimeout))
	}

	// Warn about potentially problematic timeouts
	if cfg.Timeouts.TaskCleanupInterval > 24*time.Hour {
		v.warnings = append(v.warnings, fmt.Sprintf("task_cleanup_interval very long: %v", cfg.Timeouts.TaskCleanupInterval))
	}
	if cfg.Timeouts.AuthCacheTTL > time.Hour {
		v.warnings = append(v.warnings, fmt.Sprintf("auth_cache_ttl very long: %v", cfg.Timeouts.AuthCacheTTL))
	}
}

func (v *ConfigValidator) validateValidatorPolicy(cfg *AppConfig) {
	if cfg == nil || cfg.Validator == nil {
		return
	}

	// Use safe access for policy type
	policyType, hasType := SafeGetString(cfg.Validator, "policy", "type")
	if hasType {
		if policyType == "default" || policyType == "dummy" {
			msg := "Using default/dummy validator policy - implement proper validation for production"
			if v.mode == ValidationModeProduction {
				v.errors = append(v.errors, msg)
			} else {
				v.warnings = append(v.warnings, msg)
			}
		}
	}
}

func (v *ConfigValidator) validateSecurity(cfg *AppConfig) {
	// Check TLS configuration
	if !cfg.TLS.Enabled && v.mode == ValidationModeProduction {
		v.errors = append(v.errors, "TLS is disabled - enable TLS for production")
	}

	// Check RPC authentication
	if !cfg.RPC.Auth.Enabled && v.mode == ValidationModeProduction {
		v.warnings = append(v.warnings, "RPC authentication is disabled - consider enabling for production")
	}

	// Check bearer tokens
	if cfg.RPC.Auth.Enabled && len(cfg.RPC.Auth.BearerTokens) == 0 {
		v.warnings = append(v.warnings, "RPC authentication enabled but no bearer tokens configured")
	}
}

func (v *ConfigValidator) validateBlockchain(cfg *AppConfig) {
	if cfg == nil || cfg.Blockchain == nil || !cfg.Blockchain.Enabled {
		return
	}

	if strings.TrimSpace(cfg.Blockchain.RPCURL) == "" {
		v.errors = append(v.errors, "blockchain.rpc_url is required when blockchain verification is enabled")
	}

	addr := cfg.Blockchain.SubnetAddress()
	if addr == "" {
		v.errors = append(v.errors, "blockchain.subnet_contract is required when blockchain verification is enabled")
	} else if !common.IsHexAddress(addr) {
		v.errors = append(v.errors, fmt.Sprintf("blockchain.subnet_contract is not a valid address: %s", addr))
	}

	if v.mode == ValidationModeProduction && cfg.Blockchain.AllowUnverified {
		v.warnings = append(v.warnings, "Allowing unverified agents while blockchain verification is enabled - consider disabling allow_unverified in production")
	}
}

func (v *ConfigValidator) validateDemoSigners(cfg *AppConfig) {
	// Check for demo signers
	if len(cfg.DemoSigners) > 0 {
		msg := fmt.Sprintf("Using %d demo signer(s) - remove for production", len(cfg.DemoSigners))
		if v.mode == ValidationModeProduction {
			v.errors = append(v.errors, msg)
		} else {
			v.warnings = append(v.warnings, msg)
		}

		// Check for weak demo keys
		for _, signer := range cfg.DemoSigners {
			if strings.Contains(signer.PrivKeyHex, "01234567") || strings.Contains(signer.PrivKeyHex, "abcdef") {
				v.warnings = append(v.warnings, fmt.Sprintf("Demo signer %s using weak example key", signer.ID))
			}
		}
	}
}

// ValidateProductionReadiness performs strict validation for production deployments
func ValidateProductionReadiness(cfg *AppConfig) error {
	validator := &ConfigValidator{mode: ValidationModeProduction}
	return validator.Validate(cfg)
}

// PrintConfigurationSummary prints a summary of the configuration
func PrintConfigurationSummary(cfg *AppConfig) {
	fmt.Println("Configuration Summary:")
	fmt.Println("======================")

	fmt.Printf("Executor: %s\n", cfg.Agent.Executor.Type)
	fmt.Printf("Matcher Strategy: %s\n", cfg.Agent.Matcher.Strategy)
	fmt.Printf("Signer Type: %s\n", cfg.Agent.Matcher.Signer.Type)

	// Validator policy
	if validator := cfg.Validator; validator != nil {
		if policy, ok := validator["policy"].(map[string]interface{}); ok {
			if policyType, ok := policy["type"].(string); ok {
				fmt.Printf("Validator Policy: %s\n", policyType)
			}
		}
	}

	// Security settings
	fmt.Printf("TLS Enabled: %v\n", cfg.TLS.Enabled)
	fmt.Printf("RPC Auth Enabled: %v\n", cfg.RPC.Auth.Enabled)
	if cfg.Blockchain != nil && cfg.Blockchain.Enabled {
		fmt.Printf("Blockchain Verification: enabled (fallback=%v, allow_unverified=%v)\n",
			cfg.Blockchain.EnableFallback,
			cfg.Blockchain.AllowUnverified,
		)
	} else {
		fmt.Println("Blockchain Verification: disabled")
	}

	// Demo signers
	if len(cfg.DemoSigners) > 0 {
		fmt.Printf("Demo Signers: %d configured\n", len(cfg.DemoSigners))
	}

	fmt.Println("======================")
}
