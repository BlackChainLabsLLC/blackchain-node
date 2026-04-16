package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"blackchain/internal/crypto"
	"blackchain/internal/mesh"
)

func main() {
	// Allow env override (supports values like "127.0.0.1:6060", "http://127.0.0.1:6060", or "https://127.0.0.1:6060")
	envAPI := strings.TrimSpace(os.Getenv("BLACKCTL_API"))
	defaultAddr := "http://127.0.0.1:6060"
	if envAPI != "" {
		defaultAddr = envAPI
	}

	addr := flag.String("addr", defaultAddr, "rpc address (host:port, http://host:port, or https://host:port), or set BLACKCTL_API")
	height := flag.Int64("height", 0, "block height (for chain block)")

	walletNew := flag.String("wallet-new", "", "create wallet")
	walletShow := flag.String("wallet-show", "", "show wallet")
	walletPath := flag.String("wallet", "", "wallet path")

	to := flag.String("to", "", "to address")
	amount := flag.Int64("amount", 0, "amount")
	fee := flag.Int64("fee", 1, "transaction fee")

	flag.Parse()
	args := flag.Args()

	// ---------------- POSitional wallet aliases ----------------
	if len(args) >= 2 && args[0] == "wallet-new" {
		w, err := crypto.LoadOrCreateWallet(args[1])
		if err != nil {
			panic(err)
		}
		fmt.Println(w.Address)
		return
	}

	if len(args) >= 2 && args[0] == "wallet-show" {
		w, err := crypto.LoadWallet(args[1])
		if err != nil {
			panic(err)
		}
		fmt.Println("address:", w.Address)
		fmt.Println("pub:", w.PubHex)
		return
	}

	if len(args) >= 2 && args[0] == "wallet-addr" {
		w, err := crypto.LoadWallet(args[1])
		if err != nil {
			panic(err)
		}
		fmt.Println(w.Address)
		return
	}

	// ---------------- FLAG wallet ops ----------------
	if *walletNew != "" {
		w, err := crypto.LoadOrCreateWallet(*walletNew)
		if err != nil {
			panic(err)
		}
		fmt.Println(w.Address)
		return
	}

	if *walletShow != "" {
		w, err := crypto.LoadWallet(*walletShow)
		if err != nil {
			panic(err)
		}
		fmt.Println("address:", w.Address)
		fmt.Println("pub:", w.PubHex)
		return
	}

	// ---------------- COMMAND ROUTER ----------------
	if len(args) < 2 || args[0] != "chain" {
		usage()
		return
	}

	cmd := args[1]

	base := normalizeAPIBase(*addr)
	client := http.Client{Timeout: 12 * time.Second}
	if strings.HasPrefix(base, "https://") {
		client = http.Client{
			Timeout:   12 * time.Second,
			Transport: &http.Transport{TLSClientConfig: mesh.InsecureBlackctlTLSConfig()},
		}
	}

	// ---------------- CHAIN HEIGHT ----------------
	if cmd == "height" {
		r, err := client.Get(base + "/chain/height")
		if err != nil {
			panic(err)
		}
		defer r.Body.Close()
		out, _ := io.ReadAll(r.Body)
		fmt.Println(string(out))
		return
	}

	// ---------------- CHAIN STATUS ----------------
	if cmd == "status" {
		r, err := client.Get(base + "/chain/status")
		if err != nil {
			panic(err)
		}
		defer r.Body.Close()
		out, _ := io.ReadAll(r.Body)
		fmt.Println(string(out))
		return
	}

	// ---------------- MESH PEERS ----------------
	if cmd == "peers" {
		r, err := client.Get(base + "/peers")
		if err != nil {
			panic(err)
		}
		defer r.Body.Close()
		out, _ := io.ReadAll(r.Body)
		fmt.Println(string(out))
		return
	}

	// ---------------- TOPOLOGY ----------------
	if cmd == "topology" {
		r, err := client.Get(base + "/peers")
		if err != nil {
			panic(err)
		}
		defer r.Body.Close()
		body, _ := io.ReadAll(r.Body)
		fmt.Println("BlackChain Network Topology")
		fmt.Println("==========================")
		fmt.Println(string(body))
		return
	}

	// ---------------- CHAIN TX ----------------
	if cmd == "tx" {
		// Fallback: extract --wallet, --to, --amount, --fee from args if flags missing
		for i := 0; i < len(args)-1; i++ {
			if *walletPath == "" && args[i] == "--wallet" {
				*walletPath = args[i+1]
			}
			if *to == "" && args[i] == "--to" {
				*to = args[i+1]
			}
			if *amount == 0 && args[i] == "--amount" {
				fmt.Sscan(args[i+1], amount)
			}
			if *fee == 1 && args[i] == "--fee" {
				fmt.Sscan(args[i+1], fee)
			}
		}

		if *walletPath == "" {
			panic("missing --wallet")
		}
		if *to == "" {
			panic("missing --to")
		}
		if *amount <= 0 {
			panic("bad --amount")
		}
		if *fee < 0 {
			panic("bad --fee")
		}

		w, err := crypto.LoadWallet(*walletPath)
		if err != nil {
			panic(err)
		}

		// fetch nonce from balances (authoritative)
		br, err := client.Get(base + "/chain/balances")
		if err != nil {
			panic(err)
		}
		defer br.Body.Close()

		var bals map[string]struct {
			Balance int64 `json:"balance"`
			Nonce   int64 `json:"nonce"`
		}

		_ = json.NewDecoder(br.Body).Decode(&bals)
		acct := bals[w.Address]

		tx, err := mesh.SignTx(
			w.Address,
			*to,
			*amount,
			acct.Nonce,
			*fee,
			w.PubHex,
			w.PrivHex,
		)
		if err != nil {
			panic(err)
		}

		raw, _ := json.Marshal(tx)

		r, err := client.Post(
			base+"/chain/tx",
			"application/json",
			bytes.NewReader(raw),
		)
		if err != nil {
			panic(err)
		}
		defer r.Body.Close()

		out, _ := io.ReadAll(r.Body)
		fmt.Println(string(out))
		return
	}

	// ---------------- CHAIN PROPOSE ----------------
	if cmd == "propose" {
		r, err := client.Post(base+"/chain/propose", "application/json", bytes.NewReader([]byte("{}")))
		if err != nil {
			panic(err)
		}
		defer r.Body.Close()
		out, _ := io.ReadAll(r.Body)
		fmt.Println(string(out))
		return
	}

	// ---------------- CHAIN BLOCK ----------------
	if cmd == "block" {
		h := *height
		if h == 0 && len(args) >= 3 {
			// allow: blackctl chain block 1
			fmt.Sscan(args[2], &h)
		}
		if h <= 0 {
			panic("missing block height (use: chain block --height 1 OR chain block 1)")
		}

		// Try query form first
		url1 := fmt.Sprintf("%s/chain/block?height=%d", base, h)
		r, err := client.Get(url1)
		if err != nil {
			panic(err)
		}
		defer r.Body.Close()

		if r.StatusCode == 404 {
			// fallback: path form
			url2 := fmt.Sprintf("%s/chain/block/%d", base, h)
			r2, err := client.Get(url2)
			if err != nil {
				panic(err)
			}
			defer r2.Body.Close()
			out2, _ := io.ReadAll(r2.Body)
			fmt.Println(string(out2))
			return
		}

		out, _ := io.ReadAll(r.Body)
		fmt.Println(string(out))
		return
	}

	usage()
}

func usage() {
	fmt.Println("usage:")
	fmt.Println(" blackctl wallet-new wallet.json")
	fmt.Println(" blackctl wallet-show wallet.json")
	fmt.Println(" blackctl wallet-addr wallet.json")
	fmt.Println(" blackctl chain height")
	fmt.Println(" blackctl chain status")
	fmt.Println(" blackctl chain peers")
	fmt.Println(" blackctl chain topology")
	fmt.Println(" blackctl chain tx --wallet wallet.json --to <addr> --amount 5 --fee 1")
	fmt.Println(" blackctl chain propose")
	fmt.Println(" blackctl chain block --height 1    (or: blackctl chain block 1)")
	fmt.Println("")
	fmt.Println("env:")
	fmt.Println("  BLACKCTL_API=https://127.0.0.1:6060  (or http://127.0.0.1:6060 or 127.0.0.1:6060)")
}

func normalizeAPIBase(addr string) string {
	base := strings.TrimSpace(addr)
	if strings.HasPrefix(base, "http://") || strings.HasPrefix(base, "https://") {
		return base
	}
	return "http://" + base
}
