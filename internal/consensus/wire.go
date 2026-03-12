package consensus

import (
	"crypto/ed25519"
	"log"
)

// WireBundle holds runtime objects needed for Phase 3 finalization.
type WireBundle struct {
	ValidatorID string
	Pub         ed25519.PublicKey
	Priv        ed25519.PrivateKey
	Finalizer   *Finalizer
}

// InitConsensusFinality creates/loads validator key and finalizer.
// `validatorSet` is a list of validator IDs. MVP default: single validator = self.
func InitConsensusFinality(dataDir string, validatorSet []string) (*WireBundle, error) {
	vk, pub, priv, err := LoadOrCreateValidatorKey(dataDir)
	if err != nil {
		return nil, err
	}
	if len(validatorSet) == 0 {
		validatorSet = []string{vk.ID}
	}
	fin, err := NewFinalizer(dataDir, validatorSet)
	if err != nil {
		return nil, err
	}
	log.Printf("[consensus] validator_id=%s total_validators=%d threshold=%d", vk.ID, len(validatorSet), fin.threshold)
	return &WireBundle{
		ValidatorID: vk.ID,
		Pub:         pub,
		Priv:        priv,
		Finalizer:   fin,
	}, nil
}
