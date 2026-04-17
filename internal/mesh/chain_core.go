package mesh

import (
	"blackchain/internal/crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Phase 5.x: fee policy (deterministic, congestion-aware)
const (
	BaseFee                   int64 = 1
	FeeCongestionStep               = 256
	FeeStepIncrement          int64 = 1
	maxProposalFutureSkew           = 2 * time.Minute
	maxProposalBufferDistance int64 = 2
	MaxMempoolTxs             int   = 256
	MaxBlockTxs               int   = 64
	maxTxFieldLen             int   = 256
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

// ========================================
// PHASE 5.5 TREASURY SCAFFOLD (INACTIVE)
// ========================================

const (
	FeeSplitForkHeight int64 = 1279

	ProducerFeeBPS int64 = 9000
	TreasuryFeeBPS int64 = 1000
)

var TreasuryAddr = "TREASURY_BLACKCHAIN"

func feeSplitActive(height int64) bool {
	return height >= FeeSplitForkHeight
}

func splitFees(total int64) (int64, int64) {
	if total <= 0 {
		return 0, 0
	}
	producer := (total * ProducerFeeBPS) / 10000
	treasury := total - producer
	return producer, treasury
}

// ========================================

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

	blocks             map[int64]Block
	accounts           map[string]*Account
	mempool            []Tx
	mempoolIDs         map[string]struct{}
	mempoolSenderNonce map[string]string

	pending    map[int64]Block
	voteCounts map[int64]int

	votes map[int64]map[string]Vote

	finalizedHeight int64

	validatorSet map[string]struct{}
}

func newProductionChain() *ProductionChain {
	return &ProductionChain{
		blocks:             make(map[int64]Block),
		accounts:           make(map[string]*Account),
		mempool:            make([]Tx, 0, 256),
		mempoolIDs:         make(map[string]struct{}),
		mempoolSenderNonce: make(map[string]string),
		pending:            make(map[int64]Block),
		voteCounts:         make(map[int64]int),
		votes:              make(map[int64]map[string]Vote),
		validatorSet:       make(map[string]struct{}),
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
	if tx.Nonce < 0 {
		return fmt.Errorf("nonce < 0")
	}
	if len(tx.From) > maxTxFieldLen || len(tx.To) > maxTxFieldLen || len(tx.PubKey) > maxTxFieldLen*4 || len(tx.Signature) > maxTxFieldLen*4 {
		return fmt.Errorf("field too large")
	}
	if tx.Fee < 0 {
		return fmt.Errorf("fee < 0")
	}

	reqFee := c.requiredFeeLocked()
	if tx.Fee < reqFee {
		return fmt.Errorf("fee too low (have %d need %d)", tx.Fee, reqFee)
	}

	from := c.getAccount(tx.From)
	if from.Balance < tx.Amount+tx.Fee {
		return fmt.Errorf("insufficient balance")
	}
	if tx.Nonce != from.Nonce {
		return fmt.Errorf("bad nonce (have %d want %d)", tx.Nonce, from.Nonce)
	}

	return nil
}

func txID(tx Tx) string {
	raw, _ := json.Marshal(tx)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func senderNonceKey(tx Tx) string {
	return tx.From + ":" + fmtInt(tx.Nonce)
}

func (c *ProductionChain) resetMempoolLocked(capHint int) {
	if capHint <= 0 {
		capHint = MaxMempoolTxs
	}
	c.mempool = make([]Tx, 0, capHint)
	c.mempoolIDs = make(map[string]struct{}, capHint)
	c.mempoolSenderNonce = make(map[string]string, capHint)
}

func (c *ProductionChain) rebuildMempoolIndexLocked() {
	c.mempoolIDs = make(map[string]struct{}, len(c.mempool))
	c.mempoolSenderNonce = make(map[string]string, len(c.mempool))
	for _, tx := range c.mempool {
		id := txID(tx)
		c.mempoolIDs[id] = struct{}{}
		c.mempoolSenderNonce[senderNonceKey(tx)] = id
	}
}

func (c *ProductionChain) addTxToMempoolLocked(tx Tx) error {
	if err := c.validateTx(tx); err != nil {
		return err
	}
	if c.mempoolIDs == nil || c.mempoolSenderNonce == nil {
		c.rebuildMempoolIndexLocked()
	}
	if len(c.mempool) >= MaxMempoolTxs {
		return fmt.Errorf("mempool full (max %d)", MaxMempoolTxs)
	}
	id := txID(tx)
	if _, ok := c.mempoolIDs[id]; ok {
		return fmt.Errorf("duplicate tx")
	}
	sn := senderNonceKey(tx)
	if existing, ok := c.mempoolSenderNonce[sn]; ok && existing != id {
		return fmt.Errorf("conflicting sender nonce already pending")
	}
	c.mempool = append(c.mempool, tx)
	c.mempoolIDs[id] = struct{}{}
	c.mempoolSenderNonce[sn] = id
	return nil
}

func (c *ProductionChain) compactMempoolLocked() int {
	if len(c.mempool) == 0 {
		c.resetMempoolLocked(MaxMempoolTxs)
		return 0
	}

	kept := make([]Tx, 0, len(c.mempool))
	pendingIDs := make(map[string]struct{}, len(c.mempool))
	pendingSenderNonce := make(map[string]struct{}, len(c.mempool))
	type shadow struct {
		balance int64
		nonce   int64
	}
	shadows := make(map[string]shadow)
	getShadow := func(addr string) shadow {
		if s, ok := shadows[addr]; ok {
			return s
		}
		ac := c.getAccount(addr)
		s := shadow{balance: ac.Balance, nonce: ac.Nonce}
		shadows[addr] = s
		return s
	}

	dropped := 0
	for _, tx := range c.mempool {
		if err := c.validateTx(tx); err != nil {
			dropped++
			continue
		}
		id := txID(tx)
		if _, ok := pendingIDs[id]; ok {
			dropped++
			continue
		}
		sn := senderNonceKey(tx)
		if _, ok := pendingSenderNonce[sn]; ok {
			dropped++
			continue
		}
		from := getShadow(tx.From)
		if tx.Nonce != from.nonce {
			dropped++
			continue
		}
		if from.balance < tx.Amount+tx.Fee {
			dropped++
			continue
		}
		from.balance -= tx.Amount + tx.Fee
		from.nonce++
		shadows[tx.From] = from
		if tx.To != "" {
			to := getShadow(tx.To)
			to.balance += tx.Amount
			shadows[tx.To] = to
		}
		kept = append(kept, tx)
		pendingIDs[id] = struct{}{}
		pendingSenderNonce[sn] = struct{}{}
	}

	c.mempool = kept
	c.rebuildMempoolIndexLocked()
	return dropped
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
	if len(b.Txs) > MaxBlockTxs {
		return fmt.Errorf("block tx count exceeds max %d", MaxBlockTxs)
	}

	totalFees := int64(0)
	seenTxIDs := make(map[string]struct{}, len(b.Txs))
	seenSenderNonce := make(map[string]struct{}, len(b.Txs))

	for _, tx := range b.Txs {
		if !VerifyTxSignature(tx) {
			return fmt.Errorf("tx invalid: bad signature")
		}
		if len(tx.From) > maxTxFieldLen || len(tx.To) > maxTxFieldLen || len(tx.PubKey) > maxTxFieldLen*4 || len(tx.Signature) > maxTxFieldLen*4 {
			return fmt.Errorf("tx invalid: field too large")
		}
		id := txID(tx)
		if _, ok := seenTxIDs[id]; ok {
			return fmt.Errorf("tx invalid: duplicate tx in block")
		}
		seenTxIDs[id] = struct{}{}
		sn := senderNonceKey(tx)
		if _, ok := seenSenderNonce[sn]; ok {
			return fmt.Errorf("tx invalid: duplicate sender nonce in block")
		}
		seenSenderNonce[sn] = struct{}{}
		totalFees += tx.Fee
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

	c.observeValidatorLocked(b.ValidatorID)
	c.observeValidatorLocked(b.Producer)

	wantHash := c.calcBlockHash(b)
	if b.Hash == "" || b.Hash != wantHash {
		return fmt.Errorf("bad hash")
	}

	if err := ValidateBlockSignature(b); err != nil {
		return fmt.Errorf("invalid block signature: %w", err)
	}

	expectedReward := func() int64 {
		if feeSplitActive(b.Height) {
			producerFees, _ := splitFees(totalFees)
			return blockRewardForHeight(b.Height) + producerFees
		}
		return blockRewardForHeight(b.Height) + totalFees
	}()
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
	if err := c.persistBlockLocked(b); err != nil {
		return fmt.Errorf("persist block height %d: %w", b.Height, err)
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
		if tx.Nonce != f.nonce {
			return fmt.Errorf("tx invalid: bad nonce sequencing")
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
	if feeSplitActive(b.Height) {
		_, treasuryFees := splitFees(totalFees)
		if treasuryFees > 0 && TreasuryAddr != "" {
			treas := c.getOrCreateAccount(TreasuryAddr)
			treas.Balance += treasuryFees
		}
	}

	c.blocks[b.Height] = b
	c.height = b.Height
	c.tip = b.Hash
	c.compactMempoolLocked()

	c.advanceFinalityLocked()
	c.tip = b.Hash
	return nil
}

func (c *ProductionChain) applyBlockOrBufferLocked(b Block) (applied bool, err error) {
	defer func() {
		if err != nil && c.daemon != nil {
			c.daemon.recordProposalFailure(err)
		}
	}()
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
		return false, fmt.Errorf("proposal rejected: invalid height=%d", b.Height)
	}

	if b.Height <= c.height {
		if old, ok := c.blocks[b.Height]; ok {
			if old.Hash == b.Hash && b.Hash != "" {
				return false, fmt.Errorf("proposal rejected: duplicate committed proposal height=%d hash=%s", b.Height, b.Hash)
			}
			return false, fmt.Errorf("proposal rejected: conflicting committed proposal height=%d existing_hash=%s incoming_hash=%s", b.Height, old.Hash, b.Hash)
		}
		return false, fmt.Errorf("proposal rejected: stale proposal height=%d local_height=%d", b.Height, c.height)
	}

	if b.TimeUTC.IsZero() {
		return false, fmt.Errorf("proposal rejected: missing time")
	}
	if b.TimeUTC.After(time.Now().UTC().Add(maxProposalFutureSkew)) {
		return false, fmt.Errorf("proposal rejected: future timestamp height=%d time=%s max_skew=%s", b.Height, b.TimeUTC.UTC().Format(time.RFC3339Nano), maxProposalFutureSkew)
	}

	if prev, ok := c.blocks[c.height]; ok && b.Height == c.height+1 && !b.TimeUTC.After(prev.TimeUTC) {
		return false, fmt.Errorf("proposal rejected: stale timestamp height=%d prev_height=%d proposal_time=%s prev_time=%s", b.Height, prev.Height, b.TimeUTC.UTC().Format(time.RFC3339Nano), prev.TimeUTC.UTC().Format(time.RFC3339Nano))
	}
	if b.Hash == "" || b.Hash != c.calcBlockHash(b) {
		return false, fmt.Errorf("proposal rejected: bad hash")
	}
	if err := ValidateBlockSignature(b); err != nil {
		return false, fmt.Errorf("proposal rejected: invalid block signature: %w", err)
	}

	for _, tx := range b.Txs {
		if tx.From == "" || tx.To == "" || tx.From == tx.To || tx.Amount <= 0 {
			return false, fmt.Errorf("proposal rejected: tx invalid")
		}
	}

	if b.Height == c.height+1 {
		if err := c.applyBlockLocked(b); err != nil {
			return false, err
		}
		c.height = b.Height
		c.tip = b.Hash
		_ = c.drainPendingLocked()
		return true, nil
	}

	if b.Height > c.height+maxProposalBufferDistance {
		return false, fmt.Errorf("proposal rejected: out-of-order proposal height=%d local_height=%d max_buffer_height=%d", b.Height, c.height, c.height+maxProposalBufferDistance)
	}

	if existing, ok := c.pending[b.Height]; ok {
		if existing.Hash == b.Hash && b.Hash != "" {
			return false, fmt.Errorf("proposal rejected: duplicate pending proposal height=%d hash=%s", b.Height, b.Hash)
		}
		return false, fmt.Errorf("proposal rejected: conflicting pending proposal height=%d existing_hash=%s incoming_hash=%s", b.Height, existing.Hash, b.Hash)
	}
	c.pending[b.Height] = b
	return false, nil
}

func (c *ProductionChain) dropPendingLocked(height int64, reason string) {
	b, ok := c.pending[height]
	if !ok {
		return
	}
	delete(c.pending, height)
	log.Printf("[proposal] dropped buffered proposal height=%d hash=%s producer=%s reason=%s", height, b.Hash, b.Producer, reason)
}

func (c *ProductionChain) drainPendingLocked() int {
	applied := 0
	for {
		next := c.height + 1
		b, ok := c.pending[next]
		if !ok {
			break
		}

		if b.PrevHash != c.tip {
			c.dropPendingLocked(next, fmt.Sprintf("stale successor prev_hash=%s local_tip=%s", b.PrevHash, c.tip))
			continue
		}
		if prev, ok := c.blocks[c.height]; ok && !b.TimeUTC.After(prev.TimeUTC) {
			c.dropPendingLocked(next, fmt.Sprintf("stale successor timestamp proposal_time=%s prev_time=%s", b.TimeUTC.UTC().Format(time.RFC3339Nano), prev.TimeUTC.UTC().Format(time.RFC3339Nano)))
			continue
		}

		delete(c.pending, next)
		if err := c.applyBlockLocked(b); err != nil {
			log.Printf("[proposal] rejected buffered proposal height=%d hash=%s: %v", next, b.Hash, err)
			continue
		}
		c.height = b.Height
		c.tip = b.Hash
		applied++
	}
	return applied
}

func (c *ProductionChain) proposalSafetyCheckLocked(localValidator string) error {
	next := c.height + 1
	if pending, ok := c.pending[next]; ok {
		return fmt.Errorf("proposal aborted: successor already buffered height=%d hash=%s producer=%s", pending.Height, pending.Hash, pending.Producer)
	}

	var blocked *Block
	for h, pending := range c.pending {
		if h <= c.height {
			continue
		}
		if pending.Producer == "" || pending.Producer == localValidator {
			continue
		}
		if blocked == nil || h < blocked.Height {
			cp := pending
			blocked = &cp
		}
	}
	if blocked != nil {
		return fmt.Errorf("proposal aborted: buffered proposal from another validator height=%d producer=%s hash=%s", blocked.Height, blocked.Producer, blocked.Hash)
	}

	return nil
}

func (c *ProductionChain) requireValidatorActionReadyLocked(nodeID string) error {
	if nodeID != "node1" {
		return fmt.Errorf("validator action rejected: not leader (node_id=%s)", nodeID)
	}
	pubHex, err := c.EnsureValidatorIdentityLocked()
	if err != nil {
		return fmt.Errorf("validator action rejected: validator identity: %w", err)
	}
	if pubHex == "" || pubHex == "ERR_NO_VALIDATOR" {
		return fmt.Errorf("validator action rejected: validator identity unavailable")
	}
	return nil
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

	reward := blockRewardForHeight(b.Height) + totalFees
	if feeSplitActive(b.Height) {
		producerFees, treasuryFees := splitFees(totalFees)
		reward = blockRewardForHeight(b.Height) + producerFees
		if treasuryFees > 0 && TreasuryAddr != "" {
			t := get(TreasuryAddr)
			t.Balance += treasuryFees
		}
	}
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

	pubHex, err := c.EnsureValidatorIdentityLocked()
	if err != nil {
		return err
	}
	privHex, err := c.ValidatorPrivHexLocked()
	if err != nil {
		return err
	}
	if pubHex == "" || privHex == "" || pubHex == "ERR_NO_VALIDATOR" {
		return fmt.Errorf("missing validator keys")
	}
	if err := c.proposalSafetyCheckLocked(pubHex); err != nil {
		return err
	}
	c.compactMempoolLocked()

	privBytes, err := hex.DecodeString(privHex)
	if err != nil {
		return fmt.Errorf("bad validator priv hex: %v", err)
	}
	priv := ed25519.PrivateKey(privBytes)

	take := len(c.mempool)
	if take > MaxBlockTxs {
		take = MaxBlockTxs
	}
	txs := append([]Tx(nil), c.mempool[:take]...)
	remainder := append([]Tx(nil), c.mempool[take:]...)
	c.mempool = remainder
	c.rebuildMempoolIndexLocked()

	totalFees := int64(0)
	for _, tx := range txs {
		totalFees += tx.Fee
	}

	b := Block{
		ProducerAddr: producerAddr,
		Height:       c.height + 1,
		PrevHash:     c.tip,
		Txs:          txs,
		Reward: func() int64 {
			h := c.height + 1
			if feeSplitActive(h) {
				producerFees, _ := splitFees(totalFees)
				return blockRewardForHeight(h) + producerFees
			}
			return blockRewardForHeight(h) + totalFees
		}(),
		Producer:    pubHex,
		ValidatorID: pubHex,
		TimeUTC:     time.Now().UTC(),
	}

	b.BalancesRoot = c.calcStateHashWithBlock(b)
	b.Hash = c.calcBlockHash(b)
	SignBlock(&b, priv)

	_, err = c.applyBlockOrBufferLocked(b)
	if err != nil {
		restore := append([]Tx{}, txs...)
		restore = append(restore, c.mempool...)
		c.mempool = restore
		c.rebuildMempoolIndexLocked()
	}
	return err
}

// ===== PHASE 9: FINALITY SNAPSHOT (READ-ONLY, SAFE) =====
func (c *ProductionChain) GetFinalitySnapshot() (int64, string, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	finalizedHeight := c.finalizedHeight
	depth := finalityDepth

	var finalizedTip string
	if finalizedHeight > 0 {
		if b, ok := c.blocks[finalizedHeight]; ok {
			finalizedTip = b.Hash
		}
	}

	return finalizedHeight, finalizedTip, depth
}
