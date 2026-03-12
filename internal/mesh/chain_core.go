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
	// Deterministic: depends only on current mempool length.
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
// Era-based minting: 1 UC per block for N eras, then 0.
const (
	inflationEraBlocks int64 = 10
	inflationEras      int64 = 2
	BlockReward        int64 = 1
)

func blockRewardForHeight(h int64) int64 {
	// PHASE 5.2 — Deterministic Inflation Schedule
	// initial reward: 50
	// halving interval: 100_000 blocks
	// minimum reward: 1
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

// ProductionChain is a simple, deterministic devnet chain for demos.
// Phase D adds "late-join" support: blocks can arrive out-of-order.
// Phase F adds deterministic validation gates:
// - strict PrevHash / Height continuity
// - deterministic block hash verification
// - per-block nonce/balance simulation (multi-tx safety)
// - reject-or-buffer rules
type ProductionChain struct {
	// Phase 3.1: validator identity (cached)
	validatorPubHex  string
	validatorPrivHex string

	daemon *meshDaemon

	mu sync.RWMutex

	// Phase G: persistence roots (set by mesh daemon)
	dataDir    string
	persistDir string
	height     int64
	tip        string

	blocks   map[int64]Block
	accounts map[string]*Account
	mempool  []Tx

	// Phase D: late-join buffer (future blocks)
	pending    map[int64]Block
	voteCounts map[int64]int

	// Phase 3.2: vote collection (height -> validatorID -> vote)
	votes map[int64]map[string]Vote

	finalizedHeight int64

	validatorSet map[string]struct{}
}

func newProductionChain() *ProductionChain {
	return &ProductionChain{
		blocks:     make(map[int64]Block),
		accounts:   make(map[string]*Account),
		mempool:    make([]Tx, 0, 256),
		pending:    make(map[int64]Block),
		voteCounts: make(map[int64]int),

		votes: make(map[int64]map[string]Vote),

		validatorSet: make(map[string]struct{}),
	}
}

// ensureGenesisLocked seeds initial balances if the chain is empty.
// This prevents "insufficient funds" on first demo txs.
func (c *ProductionChain) ensureGenesisLocked() {
	// PRODUCTION: do not seed fake dev accounts (e.g. "alice"/"bob").
	// Genesis state is empty; supply enters only via block rewards (Phase 5).
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
		// READ-ONLY: do NOT create or mutate chain state on reads.
		// Missing accounts are treated as zero until a tx/block actually creates them.
		return &Account{Balance: 0, Nonce: 0}
	}
	return acct
}

// calcBlockHash deterministically computes a block hash from canonical fields.
// IMPORTANT: receivers MUST be able to recompute exactly the same hash.
func (c *ProductionChain) calcBlockHash(b Block) string {
	type canon struct {
		ValidatorID  string `json:"validator_id"`
		BalancesRoot string `json:"balances_root"`

		Height   int64  `json:"height"`
		PrevHash string `json:"prev_hash"`
		TimeUnix int64  `json:"time_unix"`
		Producer string `json:"producer"`
		Txs      []Tx   `json:"txs"`
	}
	x := canon{
		Height:       b.Height,
		PrevHash:     b.PrevHash,
		TimeUnix:     b.TimeUTC.UTC().UnixNano(),
		Producer:     b.Producer,
		Txs:          b.Txs,
		BalancesRoot: b.BalancesRoot,

		ValidatorID: b.ValidatorID,
	}
	raw, _ := json.Marshal(x)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// validateTx is the mempool gate (single-tx, current-state).
func (c *ProductionChain) validateTx(tx Tx) error {

	// Phase 1.4: signature gate (mempool)
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

	// Phase 5.x: fee floor (mempool gate)
	if tx.Fee < 0 {
		return fmt.Errorf("fee < 0")
	}
	reqFee := c.requiredFeeLocked()
	if tx.Fee < reqFee {
		return fmt.Errorf("fee too low (have %d need %d)", tx.Fee, reqFee)
	}
	if tx.Fee < 0 {
		return fmt.Errorf("fee < 0")
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

// Phase 3.2: addVoteLocked stores a vote if valid.
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

	// Deterministic validator-set learning (from votes)
	c.observeValidatorLocked(v.ValidatorID)

	// Phase 3.3: attempt finalization once vote stored
	_ = c.tryFinalizeLocked(v.Height)

	return nil
}

// applyBlockLocked is the strict state-machine gate.
// REQUIREMENTS:
// - caller holds c.mu (write)
// - b.Height == c.height+1
// - b.PrevHash == c.tip (except genesis-like first block where tip may be empty)
// - b.Hash matches deterministic recompute
// - per-block tx simulation must succeed
func (c *ProductionChain) applyBlockLocked(b Block) error {
	// Ensure genesis exists on all nodes before validation
	c.ensureGenesisLocked()

	// Ensure canonical genesis state exists before any validation gates.

	if b.Reward < 0 {
		return fmt.Errorf("negative block reward")
	}
	// Phase 1.4: signature gate (block apply)

	totalFees := int64(0)

	for _, tx := range b.Txs {

		if !VerifyTxSignature(tx) {

			return fmt.Errorf("tx invalid: bad signature")

		}

	}

	// continuity
	if b.Height != c.height+1 {
		return fmt.Errorf("bad height (have %d want %d)", b.Height, c.height+1)
	}

	// prevhash continuity (genesis is always present; strict)
	if b.PrevHash != c.tip {
		return fmt.Errorf("bad prevhash (have %q want %q)", b.PrevHash, c.tip)
	}

	// time sanity (minimal gate)
	if b.TimeUTC.IsZero() {
		return fmt.Errorf("missing time")
	}

	// Phase 3.1: validator_id gate (identity)
	if b.ValidatorID == "" {
		return fmt.Errorf("missing validator_id")
	}

	// Deterministic validator-set learning (from committed blocks)
	c.observeValidatorLocked(b.ValidatorID)
	c.observeValidatorLocked(b.Producer)

	// deterministic hash gate
	wantHash := c.calcBlockHash(b)
	if b.Hash == "" || b.Hash != wantHash {
		return fmt.Errorf("bad hash")
	}

	// Phase 1.1: block signature gate (block apply)
	if !VerifyBlockSignature(b) {
		return fmt.Errorf("invalid block signature")
	}

	// Phase 2.2: balances_root gate (state commitment)

	// PHASE 5.1 requirements (validated, but reward is applied AFTER tx commit)
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

	// ---- per-block simulation (balances + nonces) ----
	// This avoids the classic bug: multiple txs from same account in one block.
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

	// commit simulated state
	for addr, v := range s {
		ac := c.getOrCreateAccount(addr)
		ac.Balance = v.bal
		ac.Nonce = v.nonce
	}

	// PHASE 5.1: block reward mint (coinbase) (post-commit)
	recipient := b.Producer
	if b.ProducerAddr != "" {
		recipient = b.ProducerAddr
	}
	acct := c.getOrCreateAccount(recipient)
	acct.Balance += b.Reward

	// persist block
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

// applyBlockOrBufferLocked applies the block if it's the next height.
// If it's in the future (late-join), it buffers it.
// NOTE: callers must hold c.mu (write).
func (c *ProductionChain) applyBlockOrBufferLocked(b Block) (bool, error) {
	// OBSERVE_VALIDATOR_ON_BLOCK: populate validator set from observed producers
	c.observeValidatorLocked(b.Producer)

	// FINALITY_GUARD: block reorg below finalized height
	h := b.Height
	hash := b.Hash
	if c.finalizedHeight > 0 && h <= c.finalizedHeight {
		if old, ok := c.blocks[h]; ok {
			if old.Hash != hash {
				return false, fmt.Errorf("reorg blocked below finalized height: h=%d finalized=%d", h, c.finalizedHeight)
			}
		}
	}

	// Phase Q: quorum gate (reject blocks that don't match peer majority)
	if c.daemon != nil {
		pubHex := c.ValidatorIDLocked()
		// Only gate blocks produced by OTHER validators.
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

	// drop/ignore duplicates at or below current height (already have it)
	if b.Height <= c.height {
		return false, nil
	}

	// Phase F: shallow deterministic gates even for future blocks
	if b.TimeUTC.IsZero() {
		return false, fmt.Errorf("missing time")
	}
	if b.Hash == "" || b.Hash != c.calcBlockHash(b) {
		return false, fmt.Errorf("bad hash")
	}
	// basic tx shape checks (full nonce/balance checks happen when it becomes next)
	for _, tx := range b.Txs {
		if tx.From == "" || tx.To == "" || tx.From == tx.To || tx.Amount <= 0 {
			return false, fmt.Errorf("tx invalid")
		}
	}

	// next block: strict apply
	if b.Height == c.height+1 {
		if err := c.applyBlockLocked(b); err != nil {
			return false, err
		}
		// DEVNET / PHASE Z: commit synced block immediately (no vote quorum yet)
		// NOTE: applyBlockLocked already verified state + wrote block; this just advances canonical tip.
		c.height = b.Height

		c.tip = b.Hash
		if c.daemon != nil {
			go c.daemon.gossipBlock(b, 3)
		}
		_ = c.drainPendingLocked()
		return true, nil
	}

	// future block: buffer (reject-or-buffer)
	if _, ok := c.pending[b.Height]; !ok {
		c.pending[b.Height] = b
	}
	return false, nil
}

// drainPendingLocked applies buffered blocks as long as the next height is present.
// Caller must hold c.mu (write).
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
			// if invalid, stop and re-buffer to avoid losing it
			c.pending[next] = b
			break
		}
		// DEVNET / PHASE Z: commit synced block immediately (no vote quorum yet)
		// NOTE: applyBlockLocked already verified state + wrote block; this just advances canonical tip.
		c.height = b.Height
		c.tip = b.Hash
		if c.daemon != nil {
			go c.daemon.gossipBlock(b, 3)
		}
		applied++
	}
	return applied
}

// snapshotRange returns blocks in [from,to] that exist.
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

// Phase Q: compute predicted state hash after applying block (no mutation)
func (c *ProductionChain) calcStateHashWithBlock(b Block) string {

	// PURE deterministic post-state simulation (no mutation)
	// This MUST mirror applyBlockLocked: tx simulation + coinbase reward.

	// clone accounts
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

	// apply txs (shape-only; strict validation happens in applyBlockLocked)
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

	// apply coinbase reward (post-tx) - must match applyBlockLocked
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

// FINALITY_DEPTH_LOCK: finalizedHeight trails tip by a small depth so we can reject reorgs safely.
// This is a guard layer ONLY (Phase 3 hardening), not full consensus finalization.
// Tune later when Phase 3 votes/commit threshold become authoritative.
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
	// finalization target is (tip - depth)
	target := c.height - finalityDepth
	if target < 0 {
		target = 0
	}
	if target <= c.finalizedHeight {
		return
	}

	// mark blocks finalized up to target (best-effort if gaps)
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

// Phase 3.3: commit threshold
func (c *ProductionChain) quorumNeededLocked() int {
	// Deterministic majority over validator set
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
	// Phase 4.1: persist deterministic snapshot
	_ = c.SaveSnapshotLocked()

	// Phase 4.1: deterministic snapshot on finalize (idempotent)

	c.broadcastFinalizeLocked(height)

	c.blocks[height] = b
	return true
}

func (c *ProductionChain) broadcastFinalizeLocked(height int64) {
	// Phase 3.4: propagation stub (transport wiring later)
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

// Phase 1: local block production
func (c *ProductionChain) proposeBlock() error {
	// LEGO_HARDLOCK_PRODUCERADDR: ProducerAddr must be non-empty and must point to the coinbase payout address.
	producerAddr := ""
	if c.daemon != nil {
		producerAddr = strings.TrimSpace(c.daemon.walletAddr)
	}
	// Fallback: read wallet.json from chain dataDir (so ProducerAddr survives any daemon binding weirdness).
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

	// LEGO: ProducerAddr must be the daemon wallet address (coinbase payout target)
	payoutAddr := ""
	if c.daemon != nil {
		payoutAddr = strings.TrimSpace(c.daemon.walletAddr)
	}
	if payoutAddr == "" {
		return fmt.Errorf("producer payout address is empty (daemon wallet not bound)")
	}

	// HARD GUARD: propose must only run on the daemon-owned chain (prevents shadow-chain minting)
	if c.daemon == nil {
		return fmt.Errorf("chain not bound to daemon")
	}
	// ProducerAddr fallback (production): do not rely on daemon pointer being non-nil in this call path.
	// If daemon walletAddr is missing, derive it from the node wallet on disk under chain dataDir.
	if payoutAddr == "" && strings.TrimSpace(c.dataDir) != "" {
		wp := filepath.Join(strings.TrimSpace(c.dataDir), "wallet.json")
		if w, err := crypto.LoadOrCreateWallet(wp); err == nil && w != nil {
			payoutAddr = w.Address
		}
	}

	// ensure genesis exists

	// Phase 1/3: producer identity (pubkey hex) + signing key
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
