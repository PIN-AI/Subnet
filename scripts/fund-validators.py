#!/usr/bin/env python3
"""
Fund validator accounts from the main test account.

Usage:
    ./fund-validators.py <validators_json_file> [amount_eth]

Environment variables:
    PRIVATE_KEY or TEST_PRIVATE_KEY: Private key for funding account (required)
    RPC_URL: RPC endpoint (default: https://sepolia.base.org)
"""

import json
import sys
import os
from web3 import Web3
from eth_account import Account

def main():
    # Parse command line arguments
    if len(sys.argv) < 2:
        print("Usage: ./fund-validators.py <validators_json_file> [amount_eth]")
        print("\nEnvironment variables:")
        print("  PRIVATE_KEY or TEST_PRIVATE_KEY: Private key for funding account (required)")
        print("  RPC_URL: RPC endpoint (default: https://sepolia.base.org)")
        sys.exit(1)

    validators_file = sys.argv[1]
    amount_eth = sys.argv[2] if len(sys.argv) > 2 else "0.01"

    # Get configuration from environment
    private_key = os.getenv('PRIVATE_KEY') or os.getenv('TEST_PRIVATE_KEY')
    if not private_key:
        print("❌ Error: PRIVATE_KEY or TEST_PRIVATE_KEY environment variable not set")
        sys.exit(1)

    rpc_url = os.getenv('RPC_URL', 'https://sepolia.base.org')

    # Load validator addresses
    try:
        with open(validators_file, 'r') as f:
            validators = json.load(f)
    except FileNotFoundError:
        print(f"❌ Error: Validators file not found: {validators_file}")
        sys.exit(1)
    except json.JSONDecodeError as e:
        print(f"❌ Error: Invalid JSON in {validators_file}: {e}")
        sys.exit(1)

    # Connect to RPC endpoint
    w3 = Web3(Web3.HTTPProvider(rpc_url))
    if not w3.is_connected():
        print(f"❌ Failed to connect to {rpc_url}")
        sys.exit(1)

    print(f"✅ Connected to {rpc_url}")

    # Get main account
    main_account = Account.from_key(private_key)
    main_balance = w3.eth.get_balance(main_account.address)
    print(f"📊 Main account: {main_account.address}")
    print(f"💰 Balance: {w3.from_wei(main_balance, 'ether')} ETH")

    # Get current gas price
    gas_price = w3.eth.gas_price
    print(f"⛽ Gas price: {w3.from_wei(gas_price, 'gwei')} gwei")

    # Get nonce
    nonce = w3.eth.get_transaction_count(main_account.address)

    # Transfer to each validator
    amount_wei = w3.to_wei(amount_eth, 'ether')
    tx_hashes = []

    print(f"\n🚀 Sending {amount_eth} ETH to each validator...")

    for validator in validators:
        address = validator['address']

        # Build transaction
        tx = {
            'nonce': nonce,
            'to': address,
            'value': amount_wei,
            'gas': 21000,
            'gasPrice': gas_price,
            'chainId': 84532  # Base Sepolia chain ID
        }

        # Sign transaction
        signed_tx = w3.eth.account.sign_transaction(tx, private_key)

        # Send transaction
        try:
            # Handle both old and new web3.py API
            raw_tx = signed_tx.rawTransaction if hasattr(signed_tx, 'rawTransaction') else signed_tx.raw_transaction
            tx_hash = w3.eth.send_raw_transaction(raw_tx)
            tx_hashes.append((validator['id'], tx_hash.hex()))
            print(f"  ✅ {validator['id']}: {address}")
            print(f"     TX: {tx_hash.hex()}")
            nonce += 1
        except Exception as e:
            print(f"  ❌ {validator['id']}: Failed - {e}")

    # Wait for confirmations
    print(f"\n⏳ Waiting for confirmations...")
    for validator_id, tx_hash in tx_hashes:
        try:
            receipt = w3.eth.wait_for_transaction_receipt(tx_hash, timeout=120)
            if receipt['status'] == 1:
                print(f"  ✅ {validator_id}: Confirmed (block {receipt['blockNumber']})")
            else:
                print(f"  ❌ {validator_id}: Transaction failed")
        except Exception as e:
            print(f"  ⚠️  {validator_id}: {e}")

    print("\n✅ Funding complete!")

    # Show final balances
    print("\n📊 Final validator balances:")
    for validator in validators:
        balance = w3.eth.get_balance(validator['address'])
        print(f"  {validator['id']}: {w3.from_wei(balance, 'ether')} ETH")

if __name__ == "__main__":
    main()
