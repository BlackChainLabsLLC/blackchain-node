package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	bcrypto "blackchain/internal/crypto"
	"blackchain/internal/mesh"
)

func main() {
	walletPath := flag.String("wallet", "", "wallet path")
	to := flag.String("to", "", "recipient address")
	amount := flag.Int64("amount", 0, "amount")
	nonce := flag.Int64("nonce", -1, "explicit nonce")
	fee := flag.Int64("fee", 1, "transaction fee")

	flag.Parse()

	if *walletPath == "" {
		panic("missing --wallet")
	}
	if *to == "" {
		panic("missing --to")
	}
	if *amount <= 0 {
		panic("bad --amount")
	}
	if *nonce < 0 {
		panic("bad --nonce")
	}
	if *fee < 0 {
		panic("bad --fee")
	}

	w, err := bcrypto.LoadWallet(*walletPath)
	if err != nil {
		panic(err)
	}

	tx, err := mesh.SignTx(
		w.Address,
		*to,
		*amount,
		*nonce,
		*fee,
		w.PubHex,
		w.PrivHex,
	)
	if err != nil {
		panic(err)
	}

	out, err := json.Marshal(tx)
	if err != nil {
		panic(err)
	}

	_, _ = fmt.Fprintln(os.Stdout, string(out))
}
