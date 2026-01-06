package crypto
import (
    "crypto/ed25519"
    "crypto/rand"
    "encoding/hex"
)
type Identity struct {
    Private ed25519.PrivateKey
    Public  ed25519.PublicKey
    NodeID  string
}
func NewIdentity() (*Identity, error) {
    pub, priv, err := ed25519.GenerateKey(rand.Reader)
    if err != nil { return nil, err }
    return &Identity{
        Private: priv,
        Public:  pub,
        NodeID:  hex.EncodeToString(pub)[:16],
    }, nil
}

