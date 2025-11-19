package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cometbft/cometbft/crypto/ed25519"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <config_dir> <data_dir>\n", os.Args[0])
		os.Exit(1)
	}

	configDir := os.Args[1]
	dataDir := os.Args[2]

	// Generate node key (Ed25519)
	nodePrivKey := ed25519.GenPrivKey()
	nodeKey := map[string]interface{}{
		"priv_key": map[string]string{
			"type":  "tendermint/PrivKeyEd25519",
			"value": base64.StdEncoding.EncodeToString(nodePrivKey[:]),
		},
	}

	nodeKeyJSON, _ := json.MarshalIndent(nodeKey, "", "  ")
	if err := os.WriteFile(configDir+"/node_key.json", nodeKeyJSON, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write node_key.json: %v\n", err)
		os.Exit(1)
	}

	// Generate validator key (Ed25519)
	valPrivKey := ed25519.GenPrivKey()
	valPubKey := valPrivKey.PubKey().(ed25519.PubKey)

	// Calculate address (first 20 bytes of SHA256 hash)
	hash := sha256.Sum256(valPubKey[:])
	address := hex.EncodeToString(hash[:20])

	privValidatorKey := map[string]interface{}{
		"address": address,
		"pub_key": map[string]string{
			"type":  "tendermint/PubKeyEd25519",
			"value": base64.StdEncoding.EncodeToString(valPubKey[:]),
		},
		"priv_key": map[string]string{
			"type":  "tendermint/PrivKeyEd25519",
			"value": base64.StdEncoding.EncodeToString(valPrivKey[:]),
		},
	}

	privKeyJSON, _ := json.MarshalIndent(privValidatorKey, "", "  ")
	if err := os.WriteFile(configDir+"/priv_validator_key.json", privKeyJSON, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write priv_validator_key.json: %v\n", err)
		os.Exit(1)
	}

	// Create empty validator state
	valState := map[string]interface{}{
		"height": "0",
		"round":  0,
		"step":   0,
	}
	valStateJSON, _ := json.MarshalIndent(valState, "", "  ")
	if err := os.WriteFile(dataDir+"/priv_validator_state.json", valStateJSON, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write priv_validator_state.json: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Generated CometBFT keys")
}
