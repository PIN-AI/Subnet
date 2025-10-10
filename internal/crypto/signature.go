package crypto

import (
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"

	geth "github.com/ethereum/go-ethereum/crypto"
)

// Signature abstraction over ECDSA(secp256k1).

type Scheme string

const (
	ECDSA_SECP256K1 Scheme = "ECDSA-secp256k1"
)

type Signer interface {
	Scheme() Scheme
	PublicKey() []byte
	Sign(msgHash []byte) ([]byte, error) // DER-encoded signature
}

type Verifier interface {
	Verify(pubKey []byte, msgHash []byte, derSig []byte) bool
}

// ECDSA (secp256k1) implementation using go-ethereum utilities + std ecdsa ASN.1

type ECDSASigner struct{ priv *ecdsa.PrivateKey }

func NewECDSASignerFromHex(hexkey string) (*ECDSASigner, error) {
	pk, err := geth.HexToECDSA(hexkey)
	if err != nil {
		return nil, err
	}
	return &ECDSASigner{priv: pk}, nil
}

func (s *ECDSASigner) Scheme() Scheme    { return ECDSA_SECP256K1 }
func (s *ECDSASigner) PublicKey() []byte { return geth.FromECDSAPub(&s.priv.PublicKey) }
func (s *ECDSASigner) Sign(msgHash []byte) ([]byte, error) {
	// DER ASN.1 signature over provided hash
	return ecdsa.SignASN1(rand.Reader, s.priv, msgHash)
}

// GetPrivateKeyHex returns the private key as hex string
func (s *ECDSASigner) GetPrivateKeyHex() string {
	bytes := geth.FromECDSA(s.priv)
	return fmt.Sprintf("%x", bytes)
}

// GenerateECDSAKey generates a new ECDSA key pair
func GenerateECDSAKey() (*ECDSASigner, error) {
	priv, err := geth.GenerateKey()
	if err != nil {
		return nil, err
	}
	return &ECDSASigner{priv: priv}, nil
}

type ECDSAVerifier struct{}

func (v *ECDSAVerifier) Verify(pubKey []byte, msgHash []byte, derSig []byte) bool {
	pk, err := geth.UnmarshalPubkey(pubKey)
	if err != nil {
		return false
	}
	return ecdsa.VerifyASN1(pk, msgHash, derSig)
}

// HashMessage creates a Keccak256 hash of the message
func HashMessage(message []byte) [32]byte {
	return geth.Keccak256Hash(message)
}
