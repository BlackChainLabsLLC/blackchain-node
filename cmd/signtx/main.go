package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	bcrypto "blackchain/internal/crypto"
	"blackchain/internal/mesh"
)

func main() {
	wpath := filepath.Join("data", "node1", "wallet.json")
	w, err := bcrypto.LoadOrCreateWallet(wpath)
	if err != nil {
		panic(err)
	}

	tx, err := mesh.SignTx(
		w.Address,
		"bob",
		1,  // amount
		0,  // nonce
		w.PubHex,
		w.PrivHex,
	)
	if err != nil {
		panic(err)
	}

	tx.Fee = 1

	out, _ := json.Marshal(tx)
	fmt.Println(string(out))
}
