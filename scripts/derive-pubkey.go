package main

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <private_key_hex>\n", os.Args[0])
		os.Exit(1)
	}

	privateKeyHex := os.Args[1]

	// Remove 0x prefix if present
	if len(privateKeyHex) >= 2 && privateKeyHex[:2] == "0x" {
		privateKeyHex = privateKeyHex[2:]
	}

	// Parse private key
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding private key: %v\n", err)
		os.Exit(1)
	}

	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing private key: %v\n", err)
		os.Exit(1)
	}

	// Derive public key
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error casting public key to ECDSA\n")
		os.Exit(1)
	}

	// Get public key bytes (uncompressed)
	publicKeyBytes := crypto.FromECDSAPub(publicKeyECDSA)

	fmt.Printf("%s\n", hex.EncodeToString(publicKeyBytes))
}
