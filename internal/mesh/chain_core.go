package mesh

import (
	"blackchain/internal/crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Phase 5.x: fee policy (deterministic, congestion-aware)
const (
	BaseFee           int64 = 1
	FeeCongestionStep       = 256
	FeeStepIncrement  int64 = 1
)

func (c *ProductionChain) requiredFeeLocked() int64 {
	steps := 0
	if FeeCongestionStep > 0 {
		steps = len(c.mempool) / FeeCongestionStep
	}
	return BaseFee + (int64(steps) * FeeStepIncrement)
}

type Tx struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Amount    int64  `json:"amount"`
	Nonce     int64  `json:"nonce"`
	Fee       int64  `json:"fee"`
	PubKey    string `json:"pubkey,omitempty"`
	Signature string `json:"sig,omitempty"`
}

type Block struct {
	Producer       string `json:"producer"`
	ProducerAddr   string `json:"producer_addr"`
	Reward         int64  `json:"reward"`
	ProducerPubKey string
	Signature      string
	BalancesRoot   string `json:"balances_root"`
	IsFinalized    bool   `json:"finalized"`

	ValidatorID string    `json:"validator_id"`
	Height      int64     `json:"height"`
	PrevHash    string    `json:"prev_hash"`
	Hash        string    `json:"hash"`
	TimeUTC     time.Time `json:"time_utc"`
	Txs         []Tx      `json:"txs"`
}

// Phase 5.2: Inflation schedule (deterministic block reward)
const (
	inflationEraBlocks int64 = 10
	inflationEras      int64 = 2
	BlockReward        int64 = 1
)

func blockRewardForHeight(h int64) int64 {
	if h <= 0 {
		return 0
	}
	const initialReward int64 = 50
	const halvingInterval int64 = 100_000
	const minReward int64 = 1
	era := (h - 1) / halvingInterval
	if era >= 62 {
		return minReward
	}
	reward := initialReward >> uint64(era)
	if reward < minReward {
		reward = minReward
	}
	return reward
}

type Account struct {
	Balance int64 `json:"balance"`
	Nonce   int64 `json:"nonce"`
}

type ProductionChain struct {
	validatorPubHex  string
	validatorPrivHex string

	daemon *meshDaemon

	mu sync.RWMutex

	dataDir    string
	persistDir string
	height     int64
	tip        string

	blocks   map[int64]Block
	accounts map[string]*Account
	mempool  []Tx

	pending    map[int64]Block
	voteCounts map[int64]int

	votes map[int64]map[string]Vote

	finalizedHeight int64

	validatorSet map[string]struct{}
}

func newProductionChain() *ProductionChain {
	return &ProductionChain{
		blocks:       make(map[int64]Block),
		accounts:     make(map[string]*Account),
		mempool:      make([]Tx, 0, 256),
		pending:      make(map[int64]Block),
		voteCounts:   make(map[int64]int),
		votes:        make(map[int64]map[string]Vote),
		validatorSet: make(map[string]struct{}),
	}
}

// BootstrapPeers exposes the configured bootstrap peer list so the daemon
// startup layer can dial them when the node starts.
func (c *ProductionChain) BootstrapPeers() []string {
	return LoadBootstrapPeers()
}

// ensureGenesisLocked seeds initial balances if the chain is empty.
func (c *ProductionChain) ensureGenesisLocked() {
	if c.height != 0 {
		return
	}
	if c.accounts == nil {
		c.accounts = make(map[string]*Account)
	}
}

func (c *ProductionChain) getAccount(addr string) *Account {
	acct := c.accounts[addr]
	if acct == nil {
		return &Account{Balance: 0, Nonce: 0}
	}
	return acct
}

func (c *ProductionChain) calcBlockHash(b Block) string {
	type canon struct {
		ValidatorID  string `json:"validator_id"`
		BalancesRoot string `json:"balances_root"`
		Height       int64  `json:"height"`
		PrevHash     string `json:"prev_hash"`
		TimeUnix     int64  `json:"time_unix"`
		Producer     string `json:"producer"`
		Txs          []Tx   `json:"txs"`
	}
	x := canon{
		Height:       b.Height,
		PrevHash:     b.PrevHash,
		TimeUnix:     b.TimeUTC.UTC().UnixNano(),
		Producer:     b.Producer,
		Txs:          b.Txs,
		BalancesRoot: b.BalancesRoot,
		ValidatorID:  b.ValidatorID,
	}
	raw, _ := json.Marshal(x)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (c *ProductionChain) validateTx(tx Tx) error {
	if !VerifyTxSignature(tx) {
		return fmt.Errorf("tx invalid: bad signature")
	}
	if tx.From == "" || tx.To == "" {
		return fmt.Errorf("missing from/to")
	}
	if tx.From == tx.To {
		return fmt.Errorf("from == to")
	}
	if tx.Amount <= 0 {
		return fmt.Errorf("amount <= 0")
	}
	if tx.Fee < 0 {
		return fmt.Errorf("fee < 0")
	}

	reqFee := c.requiredFeeLocked()
	if tx.Fee < reqFee {
		return fmt.Errorf("fee too low (have %d need %d)", tx.Fee, reqFee)
	}

	from := c.getOrCreateAccount(tx.From)
	if from.Balance < tx.Amount+tx.Fee {
		return fmt.Errorf("insufficient balance")
	}
	if tx.Nonce != from.Nonce {
		return fmt.Errorf("bad nonce (have %d want %d)", tx.Nonce, from.Nonce)
	}

	return nil
}

func (c *ProductionChain) addVoteLocked(v Vote) error {
	if !VerifyVote(v) {
		return fmt.Errorf("bad vote signature")
	}
	if c.votes == nil {
		c.votes = make(map[int64]map[string]Vote)
	}
	m := c.votes[v.Height]
	if m == nil {
		m = make(map[string]Vote)
		c.votes[v.Height] = m
	}
	m[v.ValidatorID] = v

	c.observeValidatorLocked(v.ValidatorID)
	_ = c.tryFinalizeLocked(v.Height)

	return nil
}

func (c *ProductionChain) applyBlockLocked(b Block) error {
	c.ensureGenesisLocked()

	if b.Reward < 0 {
		return fmt.Errorf("negative block reward")
	}

	totalFees := int64(0)

	for _, tx := range b.Txs {
		if !VerifyTxSignature(tx) {
			return fmt.Errorf("tx invalid: bad signature")
		}
	}

	if b.Height != c.height+1 {
		return fmt.Errorf("bad height (have %d want %d)", b.Height, c.height+1)
	}

	if b.PrevHash != c.tip {
		return fmt.Errorf("bad prevhash (have %q want %q)", b.PrevHash, c.tip)
	}

	if b.TimeUTC.IsZero() {
		return fmt.Errorf("missing time")
	}

	if b.ValidatorID == "" {
		return fmt.Errorf("missing validator_id")
	}

	c.observeValidatorLocked(b.ValidatorID)
	c.observeValidatorLocked(b.Producer)

	wantHash := c.calcBlockHash(b)
	if b.Hash == "" || b.Hash != wantHash {
		return fmt.Errorf("bad hash")
	}

	if !VerifyBlockSignature(b) {
		return fmt.Errorf("invalid block signature")
	}

	if b.Producer == "" {
		return fmt.Errorf("missing producer")
	}

	expectedReward := blockRewardForHeight(b.Height) + totalFees
	if b.Reward != expectedReward {
		return fmt.Errorf("bad reward: got=%d want=%d", b.Reward, expectedReward)
	}

	expectedRoot := c.calcStateHashWithBlock(b)
	if b.BalancesRoot == "" {
		return fmt.Errorf("missing balances_root")
	}
	if b.BalancesRoot != expectedRoot {
		return fmt.Errorf("bad balances_root (have %s want %s)", b.BalancesRoot, expectedRoot)
	}

	type snap struct {
		bal   int64
		nonce int64
	}
	s := make(map[string]snap, 16)

	touch := func(addr string) snap {
		v, ok := s[addr]
		if ok {
			return v
		}
		ac := c.getOrCreateAccount(addr)
		v = snap{bal: ac.Balance, nonce: ac.Nonce}
		s[addr] = v
		return v
	}
	put := func(addr string, v snap) { s[addr] = v }

	for _, tx := range b.Txs {
		if tx.From == "" || tx.To == "" {
			return fmt.Errorf("tx invalid: missing from/to")
		}
		if tx.From == tx.To {
			return fmt.Errorf("tx invalid: from == to")
		}
		if tx.Amount <= 0 {
			return fmt.Errorf("tx invalid: amount <= 0")
		}

		f := touch(tx.From)
		t := touch(tx.To)

		if tx.Fee < 0 {
			return fmt.Errorf("tx invalid: negative fee")
		}
		if f.bal < tx.Amount+tx.Fee {
			return fmt.Errorf("tx invalid: insufficient balance incl fee")
		}

		f.bal -= (tx.Amount + tx.Fee)
		f.nonce++
		t.bal += tx.Amount
		totalFees += tx.Fee

		put(tx.From, f)
		put(tx.To, t)
	}

	for addr, v := range s {
		ac := c.getOrCreateAccount(addr)
		ac.Balance = v.bal
		ac.Nonce = v.nonce
	}

	recipient := b.Producer
	if b.ProducerAddr != "" {
		recipient = b.ProducerAddr
	}
	acct := c.getOrCreateAccount(recipient)
	acct.Balance += b.Reward

	c.blocks[b.Height] = b
	_ = c.persistBlockLocked(b)

	c.height = b.Height
	c.tip = b.Hash

	if c.daemon != nil {
		go c.daemon.gossipBlock(b, 3)
	}

	c.advanceFinalityLocked()
	c.tip = b.Hash
	return nil
}

func (c *ProductionChain) applyBlockOrBufferLocked(b Block) (bool, error) {
	c.observeValidatorLocked(b.Producer)

	h := b.Height
	hash := b.Hash
	if c.finalizedHeight > 0 && h <= c.finalizedHeight {
		if old, ok := c.blocks[h]; ok {
			if old.Hash != hash {
				return false, fmt.Errorf("reorg blocked below finalized height: h=%d finalized=%d", h, c.finalizedHeight)
			}
		}
	}

	if c.daemon != nil {
		pubHex := c.ValidatorIDLocked()
		if pubHex != "" && pubHex != "ERR_NO_VALIDATOR" && b.Producer != pubHex {
			predicted := c.calcStateHashWithBlock(b)
			if !c.daemon.hasQuorum(b.Height, predicted) {
				return false, fmt.Errorf("quorum not reached for block %d", b.Height)
			}
		}
	}

	if b.Height <= 0 {
		return false, fmt.Errorf("bad height")
	}

	if b.Height <= c.height {
		return false, nil
	}

	if b.TimeUTC.IsZero() {
		return false, fmt.Errorf("missing time")
	}
	if b.Hash == "" || b.Hash != c.calcBlockHash(b) {
		return false, fmt.Errorf("bad hash")
	}

	for _, tx := range b.Txs {
		if tx.From == "" || tx.To == "" || tx.From == tx.To || tx.Amount <= 0 {
			return false, fmt.Errorf("tx invalid")
		}
	}

	if b.Height == c.height+1 {
		if err := c.applyBlockLocked(b); err != nil {
			return false, err
		}
		c.height = b.Height
		c.tip = b.Hash
		if c.daemon != nil {
			go c.daemon.gossipBlock(b, 3)
		}
		_ = c.drainPendingLocked()
		return true, nil
	}

	if _, ok := c.pending[b.Height]; !ok {
		c.pending[b.Height] = b
	}
	return false, nil
}

func (c *ProductionChain) drainPendingLocked() int {
	applied := 0
	for {
		next := c.height + 1
		b, ok := c.pending[next]
		if !ok {
			break
		}
		delete(c.pending, next)
		if err := c.applyBlockLocked(b); err != nil {
			c.pending[next] = b
			break
		}
		c.height = b.Height
		c.tip = b.Hash
		if c.daemon != nil {
			go c.daemon.gossipBlock(b, 3)
		}
		applied++
	}
	return applied
}

func (c *ProductionChain) snapshotRange(from, to int64) []Block {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if from < 1 {
		from = 1
	}
	if to <= 0 || to > c.height {
		to = c.height
	}
	if to < from {
		return nil
	}

	out := make([]Block, 0, (to-from)+1)
	for h := from; h <= to; h++ {
		if b, ok := c.blocks[h]; ok {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Height < out[j].Height })
	return out
}

func (c *ProductionChain) calcStateHashWithBlock(b Block) string {
	tmp := make(map[string]*Account, len(c.accounts))
	for k, v := range c.accounts {
		vv := *v
		tmp[k] = &vv
	}

	get := func(addr string) *Account {
		a := tmp[addr]
		if a == nil {
			a = &Account{}
			tmp[addr] = a
		}
		return a
	}

	totalFees := int64(0)
	for _, tx := range b.Txs {
		if tx.From == "" || tx.To == "" || tx.From == tx.To || tx.Amount <= 0 {
			continue
		}
		from := get(tx.From)
		to := get(tx.To)
		from.Balance -= (tx.Amount + tx.Fee)
		from.Nonce++
		to.Balance += tx.Amount
		totalFees += tx.Fee
	}
	_ = totalFees

	reward := blockRewardForHeight(b.Height)
	if reward > 0 {
		recipient := b.Producer
		if b.ProducerAddr != "" {
			recipient = b.ProducerAddr
		}
		if recipient != "" {
			p := get(recipient)
			p.Balance += reward
		}
	}

	return merkleBalancesRoot(tmp)
}

const finalityDepth int64 = 2

func (c *ProductionChain) observeValidatorLocked(id string) {
	if id == "" || id == "GENESIS" {
		return
	}
	if c.validatorSet == nil {
		c.validatorSet = make(map[string]struct{})
	}
	c.validatorSet[id] = struct{}{}
}

func (c *ProductionChain) advanceFinalityLocked() {
	target := c.height - finalityDepth
	if target < 0 {
		target = 0
	}
	if target <= c.finalizedHeight {
		return
	}

	for h := c.finalizedHeight + 1; h <= target; h++ {
		if b, ok := c.blocks[h]; ok {
			if !b.IsFinalized {
				b.IsFinalized = true
				c.blocks[h] = b
			}
		}
	}
	c.finalizedHeight = target
}

func (c *ProductionChain) quorumNeededLocked() int {
	n := len(c.validatorSet)
	if n == 0 {
		return 1
	}
	return (n / 2) + 1
}

func (c *ProductionChain) tryFinalizeLocked(height int64) bool {
	v := c.voteCounts[height]
	if v < c.quorumNeededLocked() {
		return false
	}
	b, ok := c.blocks[height]
	if !ok {
		return false
	}
	b.IsFinalized = true
	_ = c.SaveSnapshotLocked()
	c.broadcastFinalizeLocked(height)
	c.blocks[height] = b
	return true
}

func (c *ProductionChain) broadcastFinalizeLocked(height int64) {
	_ = height
}

func (c *ProductionChain) getOrCreateAccount(addr string) *Account {
	ac, ok := c.accounts[addr]
	if !ok {
		ac = &Account{}
		c.accounts[addr] = ac
	}
	return ac
}

func (c *ProductionChain) proposeBlock() error {
	producerAddr := ""
	if c.daemon != nil {
		producerAddr = strings.TrimSpace(c.daemon.walletAddr)
	}
	if producerAddr == "" && strings.TrimSpace(c.dataDir) != "" {
		wp := filepath.Join(strings.TrimSpace(c.dataDir), "wallet.json")
		if b, err := os.ReadFile(wp); err == nil {
			var w struct {
				Address string `json:"address"`
			}
			if json.Unmarshal(b, &w) == nil {
				producerAddr = strings.TrimSpace(w.Address)
			}
		}
	}
	if producerAddr == "" {
		return fmt.Errorf("producer payout address is empty")
	}

	payoutAddr := ""
	if c.daemon != nil {
		payoutAddr = strings.TrimSpace(c.daemon.walletAddr)
	}
	if payoutAddr == "" {
		return fmt.Errorf("producer payout address is empty (daemon wallet not bound)")
	}

	if c.daemon == nil {
		return fmt.Errorf("chain not bound to daemon")
	}
	if payoutAddr == "" && strings.TrimSpace(c.dataDir) != "" {
		wp := filepath.Join(strings.TrimSpace(c.dataDir), "wallet.json")
		if w, err := crypto.LoadOrCreateWallet(wp); err == nil && w != nil {
			payoutAddr = w.Address
		}
	}

	pubHex := c.ValidatorIDLocked()
	privHex, err := c.ValidatorPrivHexLocked()
	if err != nil {
		return err
	}
	if pubHex == "" || privHex == "" || pubHex == "ERR_NO_VALIDATOR" {
		return fmt.Errorf("missing validator keys")
	}

	privBytes, err := hex.DecodeString(privHex)
	if err != nil {
		return fmt.Errorf("bad validator priv hex: %v", err)
	}
	priv := ed25519.PrivateKey(privBytes)

	txs := c.mempool
	c.mempool = nil

	b := Block{
		ProducerAddr: producerAddr,
		Height:       c.height + 1,
		PrevHash:     c.tip,
		Txs:          txs,
		Reward:       blockRewardForHeight(c.height + 1),
		Producer:     pubHex,
		ValidatorID:  pubHex,
		TimeUTC:      time.Now().UTC(),
	}

	b.BalancesRoot = c.calcStateHashWithBlock(b)
	b.Hash = c.calcBlockHash(b)
	SignBlock(&b, priv)

	_, err = c.applyBlockOrBufferLocked(b)
	return err
}
