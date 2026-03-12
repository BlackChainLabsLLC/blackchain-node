package crypto

import (
	"crypto/ed25519"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Wallet struct {
	Address string `json:"address"`
	PubHex  string `json:"pub_hex"`
	PrivHex string `json:"priv_hex"`
}

// AddressFromPubHex derives a stable address from a pubkey hex string.
// Current scheme: sha256(pubBytes) first 20 bytes, hex encoded (40 chars).
func AddressFromPubHex(pubHex string) (string, error) {
	pub, err := hex.DecodeString(pubHex)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(pub)
	addr := hex.EncodeToString(sum[:20])
	return addr, nil
}

func GenerateWallet() (*Wallet, error) {
	pub, priv, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		return nil, err
	}
	pubHex := hex.EncodeToString(pub)
	privHex := hex.EncodeToString(priv)
	addr, err := AddressFromPubHex(pubHex)
	if err != nil {
		return nil, err
	}
	return &Wallet{
		Address: addr,
		PubHex:  pubHex,
		PrivHex: privHex,
	}, nil
}

func LoadWallet(path string) (*Wallet, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var w Wallet
	if err := json.Unmarshal(b, &w); err != nil {
		return nil, err
	}
	if w.PubHex == "" || w.PrivHex == "" {
		return nil, errors.New("wallet missing pub/priv")
	}
	// Re-derive address if absent or mismatched (hard sanity).
	addr, err := AddressFromPubHex(w.PubHex)
	if err != nil {
		return nil, err
	}
	if w.Address == "" {
		w.Address = addr
	}
	if w.Address != addr {
		return nil, errors.New("wallet address does not match pubkey")
	}
	return &w, nil
}

func SaveWallet(path string, w *Wallet) error {
	if w == nil {
		return errors.New("nil wallet")
	}
	if w.PubHex == "" || w.PrivHex == "" {
		return errors.New("wallet missing pub/priv")
	}
	addr, err := AddressFromPubHex(w.PubHex)
	if err != nil {
		return err
	}
	if w.Address == "" {
		w.Address = addr
	}
	if w.Address != addr {
		return errors.New("wallet address does not match pubkey")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	b, _ := json.MarshalIndent(w, "", "  ")
	b = append(b, '\n')
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadOrCreateWallet loads wallet.json if present, otherwise creates a new one.
func LoadOrCreateWallet(path string) (*Wallet, error) {
	w, err := LoadWallet(path)
	if err == nil {
		return w, nil
	}
	// Only auto-create if file does not exist.
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	w, err = GenerateWallet()
	if err != nil {
		return nil, err
	}
	if err := SaveWallet(path, w); err != nil {
		return nil, err
	}
	return w, nil
}
