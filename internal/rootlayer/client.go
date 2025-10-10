package rootlayer

import (
	"context"
	"time"

	rootpb "rootlayer/proto"
	"subnet/internal/logging"
)

// Client defines the interface for interacting with RootLayer
type Client interface {
	// Intent operations
	GetPendingIntents(ctx context.Context, subnetID string) ([]*rootpb.Intent, error)
	StreamIntents(ctx context.Context, subnetID string) (<-chan *rootpb.Intent, error)

	// Assignment operations
	SubmitAssignment(ctx context.Context, assignment *rootpb.Assignment) error
	GetAssignment(ctx context.Context, assignmentID string) (*rootpb.Assignment, error)

	// Matching result operations
	SubmitMatchingResult(ctx context.Context, result *MatchingResult) error

	// Validator operations
	SubmitValidationBundle(ctx context.Context, bundle *rootpb.ValidationBundle) error

	// Connection management
	Connect() error
	Close() error
	IsConnected() bool
}

// MatchingResult contains the result of intent matching
type MatchingResult struct {
	IntentID       string
	WinningAgentID string
	WinningBidID   string
	Price          uint64
	MatcherID      string
	Timestamp      int64
	Signature      []byte
}

// Config contains configuration for RootLayer client
type Config struct {
	Endpoint     string
	SubnetID     string
	MatcherID    string
	RetryTimeout time.Duration
	MaxRetries   int
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		Endpoint:     "localhost:9090",
		SubnetID:     "subnet-1",
		MatcherID:    "matcher-1",
		RetryTimeout: 5 * time.Second,
		MaxRetries:   3,
	}
}

// BaseClient provides common functionality for RootLayer clients
type BaseClient struct {
	cfg       *Config
	logger    logging.Logger
	connected bool
}

// NewBaseClient creates a new base client
func NewBaseClient(cfg *Config, logger logging.Logger) *BaseClient {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}

	return &BaseClient{
		cfg:    cfg,
		logger: logger,
	}
}

// IsConnected returns whether the client is connected
func (c *BaseClient) IsConnected() bool {
	return c.connected
}