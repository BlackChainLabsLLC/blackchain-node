package p2p

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "errors"
    "io"
    "sync"

    "golang.org/x/crypto/curve25519"
)

// nodeCrypto manages ECC DH keys and shared secrets.
type nodeCrypto struct {
    SelfPrivateKey [32]byte
    SelfPublicKey  string

    // Peers: peerID => base64(pubkey)
    Peers map[string]string

    shared map[string][]byte
    mu     sync.Mutex
}

func newNodeCrypto() (*nodeCrypto, error) {
    var priv [32]byte
    if _, err := rand.Read(priv[:]); err != nil {
        return nil, err
    }

    var pub [32]byte
    curve25519.ScalarBaseMult(&pub, &priv)

    nc := &nodeCrypto{
        SelfPrivateKey: priv,
        SelfPublicKey:  base64.StdEncoding.EncodeToString(pub[:]),
        Peers:          map[string]string{},
        shared:         map[string][]byte{},
    }

    return nc, nil
}

// RegisterPeerPublicKey stores a peerID => pubkey entry.
func (nc *nodeCrypto) RegisterPeerPublicKey(peerID string, pubKeyBase64 string) error {
    nc.mu.Lock()
    defer nc.mu.Unlock()

    _, err := base64.StdEncoding.DecodeString(pubKeyBase64)
    if err != nil {
        return err
    }

    nc.Peers[peerID] = pubKeyBase64
    return nil
}

func deriveKey(priv [32]byte, peerPubBase64 string) ([]byte, error) {
    pubBytes, err := base64.StdEncoding.DecodeString(peerPubBase64)
    if err != nil {
        return nil, err
    }
    if len(pubBytes) != 32 {
        return nil, errors.New("invalid peer public key length")
    }

    var peerPub [32]byte
    copy(peerPub[:], pubBytes)

    var shared [32]byte
    curve25519.ScalarMult(&shared, &priv, &peerPub)

    h := sha256.Sum256(shared[:])
    return h[:], nil
}

// ensureSharedKey computes & caches the shared secret for peerID.
func (nc *nodeCrypto) ensureSharedKey(peerID string) ([]byte, error) {
    nc.mu.Lock()
    defer nc.mu.Unlock()

    if k, ok := nc.shared[peerID]; ok {
        return k, nil
    }
    pubBase64, ok := nc.Peers[peerID]
    if !ok {
        return nil, errors.New("no pubkey for peer")
    }

    key, err := deriveKey(nc.SelfPrivateKey, pubBase64)
    if err != nil {
        return nil, err
    }

    nc.shared[peerID] = key
    return key, nil
}

// EncryptForPeer encrypts using AES-GCM with the DH-derived shared key.
func (nc *nodeCrypto) EncryptForPeer(peerID string, plaintext []byte) (string, string, error) {
    key, err := nc.ensureSharedKey(peerID)
    if err != nil {
        return "", "", err
    }

    block, err := aes.NewCipher(key)
    if err != nil {
        return "", "", err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", "", err
    }

    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", "", err
    }

    ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

    return base64.StdEncoding.EncodeToString(nonce),
        base64.StdEncoding.EncodeToString(ciphertext),
        nil
}

// DecryptFromPeer decrypts AES-GCM output.
func (nc *nodeCrypto) DecryptFromPeer(peerID, nonceB64, cipherB64 string) ([]byte, error) {
    key, err := nc.ensureSharedKey(peerID)
    if err != nil {
        return nil, err
    }

    nonce, err := base64.StdEncoding.DecodeString(nonceB64)
    if err != nil {
        return nil, err
    }
    cipherBytes, err := base64.StdEncoding.DecodeString(cipherB64)
    if err != nil {
        return nil, err
    }

    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }

    plain, err := gcm.Open(nil, nonce, cipherBytes, nil)
    if err != nil {
        return nil, err
    }

    return plain, nil
}

