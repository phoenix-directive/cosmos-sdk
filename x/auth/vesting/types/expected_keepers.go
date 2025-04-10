package types

import (
	context "context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// BankKeeper defines the expected interface contract the vesting module requires
// for creating vesting accounts with funds.
type BankKeeper interface {
	IsSendEnabledCoins(ctx context.Context, coins ...sdk.Coin) error
	SendCoins(ctx context.Context, fromAddr, toAddr sdk.AccAddress, amt sdk.Coins) error
	BlockedAddr(addr sdk.AccAddress) bool
}

// DistrKeeper defines the expected interface for distribution keeper
type DistrKeeper interface {
	FundCommunityPool(ctx context.Context, amount sdk.Coins, sender sdk.AccAddress) error
}

// StakingKeeper defines the exported interface for staking keeper
type StakingKeeper interface {
	GetDelegatorDelegations(ctx context.Context, delegator sdk.AccAddress, maxRetrieve uint16) (delegations []stakingtypes.Delegation, err error)
	GetUnbondingDelegations(ctx context.Context, delegator sdk.AccAddress, maxRetrieve uint16) (unbondingDelegations []stakingtypes.UnbondingDelegation, err error)
	GetRedelegations(ctx context.Context, delegator sdk.AccAddress, maxRetrieve uint16) (redelegations []stakingtypes.Redelegation, err error)

	RemoveValidatorTokensAndShares(ctx context.Context, validator stakingtypes.Validator, sharesToRemove math.LegacyDec) (valOut stakingtypes.Validator, removedTokens math.Int)
	GetValidator(ctx context.Context, addr sdk.ValAddress) (validator stakingtypes.Validator, found bool)
	RemoveDelegation(ctx context.Context, delegation stakingtypes.Delegation) error
}
