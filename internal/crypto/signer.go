package crypto

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"time"

	geth "github.com/ethereum/go-ethereum/crypto"
)

// ExtendedSigner extends the basic Signer interface with additional methods
type ExtendedSigner interface {
	Signer

	// SignMessage signs a message after hashing it
	SignMessage(message []byte) ([]byte, error)

	// GetAddress returns the address/ID of the signer
	GetAddress() string

	// GetPublicKey returns the public key bytes
	GetPublicKey() []byte
}

// ExtendedVerifier extends the basic Verifier interface
type ExtendedVerifier interface {
	Verifier

	// VerifyMessage verifies a signature against a message
	VerifyMessage(pubKey []byte, message []byte, signature []byte) bool

	// RecoverPubKey recovers the public key from signature (if supported)
	RecoverPubKey(hash []byte, signature []byte) ([]byte, error)
}

// SignerWrapper wraps a basic Signer to provide extended functionality
type SignerWrapper struct {
	signer Signer
}

// NewSignerWrapper creates a new signer wrapper
func NewSignerWrapper(signer Signer) ExtendedSigner {
	return &SignerWrapper{signer: signer}
}

// Sign implements Signer interface
func (sw *SignerWrapper) Sign(msgHash []byte) ([]byte, error) {
	return sw.signer.Sign(msgHash)
}

// Scheme implements Signer interface
func (sw *SignerWrapper) Scheme() Scheme {
	return sw.signer.Scheme()
}

// PublicKey implements Signer interface
func (sw *SignerWrapper) PublicKey() []byte {
	return sw.signer.PublicKey()
}

// SignMessage signs a message after hashing it
func (sw *SignerWrapper) SignMessage(message []byte) ([]byte, error) {
	hash := HashMessage(message)
	return sw.signer.Sign(hash[:])
}

// GetAddress returns the address/ID of the signer
func (sw *SignerWrapper) GetAddress() string {
	pubKey := sw.signer.PublicKey()
	// Use Ethereum-style address
	pubKeyECDSA, err := geth.UnmarshalPubkey(pubKey)
	if err != nil {
		return "invalid"
	}
	return geth.PubkeyToAddress(*pubKeyECDSA).Hex()
}

// GetPublicKey returns the public key bytes
func (sw *SignerWrapper) GetPublicKey() []byte {
	return sw.signer.PublicKey()
}

// ExtendedECDSASigner wraps ECDSASigner with extended functionality
type ExtendedECDSASigner struct {
	*ECDSASigner
}

// NewExtendedECDSASigner creates a new extended ECDSA signer
func NewExtendedECDSASigner(privKey *ecdsa.PrivateKey) ExtendedSigner {
	return &ExtendedECDSASigner{
		ECDSASigner: &ECDSASigner{priv: privKey},
	}
}

// SignMessage signs a message after hashing it
func (es *ExtendedECDSASigner) SignMessage(message []byte) ([]byte, error) {
	hash := HashMessage(message)
	return es.Sign(hash[:])
}

// GetAddress returns the Ethereum-style address
func (es *ExtendedECDSASigner) GetAddress() string {
	if es.ECDSASigner == nil || es.ECDSASigner.priv == nil {
		return "invalid"
	}
	return geth.PubkeyToAddress(es.ECDSASigner.priv.PublicKey).Hex()
}

// GetPublicKey returns the public key bytes
func (es *ExtendedECDSASigner) GetPublicKey() []byte {
	return es.PublicKey()
}

// ECDSAVerifierExtended provides extended verification functionality
type ECDSAVerifierExtended struct {
	*ECDSAVerifier
}

// NewExtendedVerifier creates a new extended verifier
func NewExtendedVerifier() ExtendedVerifier {
	return &ECDSAVerifierExtended{
		ECDSAVerifier: &ECDSAVerifier{},
	}
}

// VerifyMessage verifies a signature against a message
func (ev *ECDSAVerifierExtended) VerifyMessage(pubKey []byte, message []byte, signature []byte) bool {
	hash := HashMessage(message)
	return ev.Verify(pubKey, hash[:], signature)
}

// RecoverPubKey recovers public key from signature (Ethereum-style)
func (ev *ECDSAVerifierExtended) RecoverPubKey(hash []byte, signature []byte) ([]byte, error) {
	// This would use Ethereum-style signature recovery
	// For now, return an error as it requires special signature format
	return nil, fmt.Errorf("signature recovery not implemented for DER signatures")
}

// SignerManager manages signers for different components
type SignerManager struct {
	signers map[string]ExtendedSigner
}

// NewSignerManager creates a new signer manager
func NewSignerManager() *SignerManager {
	return &SignerManager{
		signers: make(map[string]ExtendedSigner),
	}
}

// AddSigner adds a signer for a component
func (sm *SignerManager) AddSigner(component string, signer ExtendedSigner) {
	sm.signers[component] = signer
}

// GetSigner gets the signer for a component
func (sm *SignerManager) GetSigner(component string) (ExtendedSigner, error) {
	signer, exists := sm.signers[component]
	if !exists {
		return nil, fmt.Errorf("no signer found for component: %s", component)
	}
	return signer, nil
}

// MustGetSigner gets the signer for a component, panics if not found
func (sm *SignerManager) MustGetSigner(component string) ExtendedSigner {
	signer, err := sm.GetSigner(component)
	if err != nil {
		panic(fmt.Sprintf("CRITICAL: %v - system cannot operate without signer", err))
	}
	return signer
}

// ValidateSignerConfig validates that all required signers are configured
func (sm *SignerManager) ValidateSignerConfig(requiredComponents []string) error {
	var missingComponents []string

	for _, component := range requiredComponents {
		if _, err := sm.GetSigner(component); err != nil {
			missingComponents = append(missingComponents, component)
		}
	}

	if len(missingComponents) > 0 {
		return fmt.Errorf("missing signers for components: %v", missingComponents)
	}

	return nil
}

// LoadSignerFromPrivateKey loads a signer from a private key
func LoadSignerFromPrivateKey(privateKeyHex string) (ExtendedSigner, error) {
	if privateKeyHex == "" {
		return nil, errors.New("private key is empty")
	}

	// Parse the private key
	privKey, err := geth.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return NewExtendedECDSASigner(privKey), nil
}

// LoadSignerFromKeyFile loads a signer from a key file
func LoadSignerFromKeyFile(keyFile string) (ExtendedSigner, error) {
	if keyFile == "" {
		return nil, errors.New("key file path is empty")
	}

	// Load private key from file
	privKey, err := geth.LoadECDSA(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key from file: %w", err)
	}

	return NewExtendedECDSASigner(privKey), nil
}

// MustLoadSigner loads a signer and panics if it fails (for required signers)
func MustLoadSigner(privateKeyHex string, component string) ExtendedSigner {
	signer, err := LoadSignerFromPrivateKey(privateKeyHex)
	if err != nil {
		panic(fmt.Sprintf("CRITICAL: Failed to load signer for %s: %v", component, err))
	}
	return signer
}

// SignatureMetadata contains metadata about a signature
type SignatureMetadata struct {
	Signer    string `json:"signer"`     // Address/ID of signer
	Timestamp int64  `json:"timestamp"`  // Unix timestamp of signing
	Nonce     string `json:"nonce"`      // Random nonce for replay protection
	ChainID   string `json:"chain_id"`   // Chain/subnet ID for cross-chain protection
}

// SignedMessage represents a message with its signature
type SignedMessage struct {
	Message   []byte            `json:"message"`
	Signature []byte            `json:"signature"`
	Metadata  SignatureMetadata `json:"metadata"`
}

// CreateSignedMessage creates a signed message with metadata
func CreateSignedMessage(message []byte, signer ExtendedSigner, chainID string, nonce string) (*SignedMessage, error) {
	// Create the full message with metadata
	metadata := SignatureMetadata{
		Signer:    signer.GetAddress(),
		Timestamp: getCurrentTimestamp(),
		Nonce:     nonce,
		ChainID:   chainID,
	}

	// Create canonical message for signing (includes metadata)
	canonicalMsg := createCanonicalMessage(message, metadata)

	// Sign the canonical message
	signature, err := signer.SignMessage(canonicalMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to sign message: %w", err)
	}

	return &SignedMessage{
		Message:   message,
		Signature: signature,
		Metadata:  metadata,
	}, nil
}

// VerifySignedMessage verifies a signed message
func VerifySignedMessage(msg *SignedMessage, verifier ExtendedVerifier, pubKey []byte) bool {
	// Recreate the canonical message
	canonicalMsg := createCanonicalMessage(msg.Message, msg.Metadata)

	// Verify the signature
	return verifier.VerifyMessage(pubKey, canonicalMsg, msg.Signature)
}

// Helper functions

func createCanonicalMessage(message []byte, metadata SignatureMetadata) []byte {
	// Create a deterministic canonical form
	// In production, use a proper encoding like protobuf or RLP
	canonical := fmt.Sprintf("%s|%s|%d|%s|%s",
		metadata.ChainID,
		metadata.Signer,
		metadata.Timestamp,
		metadata.Nonce,
		string(message),
	)
	return []byte(canonical)
}

func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}

// Global signer manager instance (optional, can be created per service)
var globalSignerManager = NewSignerManager()

// GetGlobalSignerManager returns the global signer manager
func GetGlobalSignerManager() *SignerManager {
	return globalSignerManager
}

// InitializeGlobalSigner initializes the global signer for a component
func InitializeGlobalSigner(component string, privateKeyHex string) error {
	signer, err := LoadSignerFromPrivateKey(privateKeyHex)
	if err != nil {
		return fmt.Errorf("failed to initialize signer for %s: %w", component, err)
	}

	globalSignerManager.AddSigner(component, signer)
	return nil
}

// MustInitializeGlobalSigner initializes the global signer and panics on failure
func MustInitializeGlobalSigner(component string, privateKeyHex string) {
	if err := InitializeGlobalSigner(component, privateKeyHex); err != nil {
		panic(fmt.Sprintf("CRITICAL: %v", err))
	}
}

// Constants for component names
const (
	ComponentValidator = "validator"
	ComponentMatcher   = "matcher"
	ComponentAgent     = "agent"
	ComponentRootLayer = "rootlayer"
)

// ValidateRequiredSigners validates that all required signers are present
func ValidateRequiredSigners(components ...string) error {
	return globalSignerManager.ValidateSignerConfig(components)
}

// MustValidateRequiredSigners validates required signers and panics if any are missing
func MustValidateRequiredSigners(components ...string) {
	if err := ValidateRequiredSigners(components...); err != nil {
		panic(fmt.Sprintf("CRITICAL: %v - system cannot operate without required signers", err))
	}
}