package main

import (
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

// TestVerifySignature proves the EIP-191 personal_sign verification: a genuine
// signature passes (both the 0/1 and MetaMask 27/28 recovery-id forms), while a
// wrong message or wrong address is rejected.
func TestVerifySignature(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	addr := crypto.PubkeyToAddress(key.PublicKey).Hex()
	msg := signMessage("deadbeefdeadbeef")

	sig, err := crypto.Sign(accounts.TextHash([]byte(msg)), key)
	if err != nil {
		t.Fatal(err)
	}

	if !verifySignature(addr, msg, hexutil.Encode(sig)) {
		t.Fatal("valid signature (v=0/1) rejected")
	}

	// MetaMask returns v as 27/28 — verifySignature must normalise it.
	mm := make([]byte, len(sig))
	copy(mm, sig)
	mm[64] += 27
	if !verifySignature(addr, msg, hexutil.Encode(mm)) {
		t.Fatal("valid signature (v=27/28) rejected")
	}

	if verifySignature(addr, signMessage("other-nonce"), hexutil.Encode(sig)) {
		t.Fatal("signature accepted for the wrong message")
	}

	other, _ := crypto.GenerateKey()
	if verifySignature(crypto.PubkeyToAddress(other.PublicKey).Hex(), msg, hexutil.Encode(sig)) {
		t.Fatal("signature accepted for the wrong address")
	}
}
