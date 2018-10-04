package state

import (
	"fmt"

	"github.com/thetatoken/ukulele/common"
	"github.com/thetatoken/ukulele/core"
	"github.com/thetatoken/ukulele/ledger/types"
	"github.com/thetatoken/ukulele/store/database"
)

//
// ------------------------- State -------------------------
//

var _ types.ViewDataAccessor = (*LedgerState)(nil)

type LedgerState struct {
	chainID string
	db      database.Database

	coinbaseTransactinProcessed bool
	slashIntents                []types.SlashIntent
	validatorsDiff              []*core.Validator

	delivered *StoreView // for actually applying the transactions
	checked   *StoreView // for block proposal check
	screened  *StoreView // for mempool screening
}

// NewLedgerState creates a new Leger State with given store.
// NOTE: before using the LedgerState, we need to call LedgerState.ResetState() to set
//       the proper height and stateRootHash
func NewLedgerState(chainID string, db database.Database) *LedgerState {
	s := &LedgerState{
		chainID: chainID,
		db:      db,
	}
	s.ResetState(uint32(0), common.Hash{})
	return s
}

// ResetState resets the height and state root of its storeviews, and clear the in-memory states
func (s *LedgerState) ResetState(height uint32, stateRootHash common.Hash) bool {
	storeview := NewStoreView(height, stateRootHash, s.db)
	if storeview == nil {
		panic(fmt.Sprintf("Failed to set ledger state with state root hash: %v", stateRootHash))
	}
	s.delivered = storeview

	var err error
	s.checked, err = s.delivered.Copy()
	if err != nil {
		panic(fmt.Sprintf("Failed to copy to the checked view: %v", err))
	}
	s.screened, err = s.delivered.Copy()
	if err != nil {
		panic(fmt.Sprintf("Failed to copy to the screened view: %v", err))
	}

	s.coinbaseTransactinProcessed = false
	s.slashIntents = []types.SlashIntent{}
	s.validatorsDiff = []*core.Validator{}
	return true
}

// GetChainID gets chain ID.
func (s *LedgerState) GetChainID() string {
	if s.chainID != "" {
		return s.chainID
	}
	s.chainID = string(s.delivered.Get(ChainIDKey()))
	return s.chainID
}

// Height returns the block height corresponding to the ledger state
func (s *LedgerState) Height() uint32 {
	return s.delivered.Height()
}

// AddSlashIntent adds slashIntent
func (s *LedgerState) AddSlashIntent(slashIntent types.SlashIntent) {
	s.slashIntents = append(s.slashIntents, slashIntent)
}

// GetSlashIntents retrieves all the slashIntents
func (s *LedgerState) GetSlashIntents() []types.SlashIntent {
	return s.slashIntents
}

// ClearSlashIntents clears all the slashIntents
func (s *LedgerState) ClearSlashIntents() {
	s.slashIntents = []types.SlashIntent{}
}

// CoinbaseTransactinProcessed returns whether the coinbase transaction for the current block has been processed
func (s *LedgerState) CoinbaseTransactinProcessed() bool {
	return s.coinbaseTransactinProcessed
}

// SetCoinbaseTransactionProcessed sets whether the coinbase transaction for the current block has been processed
func (s *LedgerState) SetCoinbaseTransactionProcessed(processed bool) {
	s.coinbaseTransactinProcessed = processed
}

// GetAndClearValidatorDiff retrives and clear validator diff
func (s *LedgerState) GetAndClearValidatorDiff() []*core.Validator {
	res := s.validatorsDiff
	s.validatorsDiff = nil
	return res
}

// SetValidatorDiff set validator diff
func (s *LedgerState) SetValidatorDiff(diff []*core.Validator) {
	s.validatorsDiff = diff
}

// Delivered returns a view of current state that contains both committed and delivered
// transcations.
func (s *LedgerState) Delivered() *StoreView {
	return s.delivered
}

// Checked creates a fresh clone of delivered view to be used for checking transcations.
func (s *LedgerState) Checked() *StoreView {
	return s.checked
}

// Screened creates a fresh clone of delivered view to be used for checking transcations.
func (s *LedgerState) Screened() *StoreView {
	return s.screened
}

// Commit stores the current delivered view as committed, starts new delivered/checked state and
// returns the hash for the commit.
func (s *LedgerState) Commit() common.Hash {
	hash := s.delivered.Save()
	s.delivered.IncrementHeight()

	var err error
	s.checked, err = s.delivered.Copy()
	if err != nil {
		panic(fmt.Errorf("Commit: failed to copy to the checked view: %v", err))
	}
	s.screened, err = s.delivered.Copy()
	if err != nil {
		panic(fmt.Errorf("Commit: failed to copy to the screened view: %v", err))
	}
	return hash
}

// GetAccount implements the ViewDataAccessor interface
func (s *LedgerState) GetAccount(addr common.Address) *types.Account {
	// return types.GetAccount(s.Delivered(), addr)
	return s.Delivered().GetAccount(addr)
}

// SetAccount implements the ViewDataAccessor interface
func (s *LedgerState) SetAccount(addr common.Address, acc *types.Account) {
	s.Delivered().SetAccount(addr, acc)
}

// SplitContractExists checks if a split contract associated with the given resourceID already exists
func (s *LedgerState) SplitContractExists(resourceID common.Bytes) bool {
	exists := (s.Delivered().GetSplitContract(resourceID) != nil)
	return exists
}

// GetSplitContract implements the ViewDataAccessor interface
func (s *LedgerState) GetSplitContract(resourceID common.Bytes) *types.SplitContract {
	return s.Delivered().GetSplitContract(resourceID)
}

// SetSplitContract implements the ViewDataAccessor interface
func (s *LedgerState) SetSplitContract(resourceID common.Bytes, splitContract *types.SplitContract) {
	s.Delivered().SetSplitContract(resourceID, splitContract)
}

// AddSplitContract adds a split contract
func (s *LedgerState) AddSplitContract(splitContract *types.SplitContract) bool {
	if s.SplitContractExists(splitContract.ResourceID) {
		return false // Each resourceID can have at most one corresponding split contract
	}

	s.SetSplitContract(splitContract.ResourceID, splitContract)
	return true
}

// UpdateSplitContract updates a split contract
func (s *LedgerState) UpdateSplitContract(splitContract *types.SplitContract) bool {
	if !s.SplitContractExists(splitContract.ResourceID) {
		return false
	}

	s.SetSplitContract(splitContract.ResourceID, splitContract)
	return true
}

// DeleteSplitContract implements the ViewDataAccessor interface
func (s *LedgerState) DeleteSplitContract(resourceID common.Bytes) bool {
	return s.Delivered().DeleteSplitContract(resourceID)
}

// DeleteExpiredSplitContracts implements the ViewDataAccessor interface
func (s *LedgerState) DeleteExpiredSplitContracts(currentBlockHeight uint32) bool {
	return s.Delivered().DeleteExpiredSplitContracts(currentBlockHeight)
}
