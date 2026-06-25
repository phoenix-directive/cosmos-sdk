package vesting

import (
	"context"
	"math"

	"github.com/hashicorp/go-metrics"

	errorsmod "cosmossdk.io/errors"

	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting/exported"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
)

type msgServer struct {
	keeper.AccountKeeper
	types.BankKeeper
	types.DistrKeeper
	types.StakingKeeper
}

// NewMsgServerImpl returns an implementation of the vesting MsgServer interface,
// wrapping the corresponding AccountKeeper and BankKeeper.
func NewMsgServerImpl(k keeper.AccountKeeper, bk types.BankKeeper, dk types.DistrKeeper, sk types.StakingKeeper) types.MsgServer {
	return &msgServer{AccountKeeper: k, BankKeeper: bk, DistrKeeper: dk, StakingKeeper: sk}
}

var _ types.MsgServer = msgServer{}

func (s msgServer) CreateVestingAccount(goCtx context.Context, msg *types.MsgCreateVestingAccount) (*types.MsgCreateVestingAccountResponse, error) {
	from, err := s.AccountKeeper.AddressCodec().StringToBytes(msg.FromAddress)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid 'from' address: %s", err)
	}

	to, err := s.AccountKeeper.AddressCodec().StringToBytes(msg.ToAddress)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid 'to' address: %s", err)
	}

	if err := validateAmount(msg.Amount); err != nil {
		return nil, err
	}

	if msg.EndTime <= 0 {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "invalid end time")
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := s.IsSendEnabledCoins(ctx, msg.Amount...); err != nil {
		return nil, err
	}

	if s.BlockedAddr(to) {
		return nil, errorsmod.Wrapf(sdkerrors.ErrUnauthorized, "%s is not allowed to receive funds", msg.ToAddress)
	}

	if acc := s.GetAccount(ctx, to); acc != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "account %s already exists", msg.ToAddress)
	}

	baseAccount := authtypes.NewBaseAccountWithAddress(to)
	baseAccount = s.NewAccount(ctx, baseAccount).(*authtypes.BaseAccount)
	baseVestingAccount, err := types.NewBaseVestingAccount(baseAccount, msg.Amount.Sort(), msg.EndTime)
	if err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, err.Error())
	}

	var vestingAccount sdk.AccountI
	if msg.Delayed {
		vestingAccount = types.NewDelayedVestingAccountRaw(baseVestingAccount)
	} else {
		vestingAccount = types.NewContinuousVestingAccountRaw(baseVestingAccount, ctx.BlockTime().Unix())
	}

	s.SetAccount(ctx, vestingAccount)

	defer func() {
		telemetry.IncrCounter(1, "new", "account")

		for _, a := range msg.Amount {
			if a.Amount.IsInt64() {
				telemetry.SetGaugeWithLabels(
					[]string{"tx", "msg", "create_vesting_account"},
					float32(a.Amount.Int64()),
					[]metrics.Label{telemetry.NewLabel("denom", a.Denom)},
				)
			}
		}
	}()

	if err = s.SendCoins(ctx, from, to, msg.Amount); err != nil {
		return nil, err
	}

	return &types.MsgCreateVestingAccountResponse{}, nil
}

func (s msgServer) CreatePermanentLockedAccount(goCtx context.Context, msg *types.MsgCreatePermanentLockedAccount) (*types.MsgCreatePermanentLockedAccountResponse, error) {
	from, err := s.AccountKeeper.AddressCodec().StringToBytes(msg.FromAddress)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid 'from' address: %s", err)
	}

	to, err := s.AccountKeeper.AddressCodec().StringToBytes(msg.ToAddress)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid 'to' address: %s", err)
	}

	if err := validateAmount(msg.Amount); err != nil {
		return nil, err
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := s.IsSendEnabledCoins(ctx, msg.Amount...); err != nil {
		return nil, err
	}

	if s.BlockedAddr(to) {
		return nil, errorsmod.Wrapf(sdkerrors.ErrUnauthorized, "%s is not allowed to receive funds", msg.ToAddress)
	}

	if acc := s.GetAccount(ctx, to); acc != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "account %s already exists", msg.ToAddress)
	}

	baseAccount := authtypes.NewBaseAccountWithAddress(to)
	baseAccount = s.NewAccount(ctx, baseAccount).(*authtypes.BaseAccount)
	vestingAccount, err := types.NewPermanentLockedAccount(baseAccount, msg.Amount)
	if err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, err.Error())
	}

	s.SetAccount(ctx, vestingAccount)

	defer func() {
		telemetry.IncrCounter(1, "new", "account")

		for _, a := range msg.Amount {
			if a.Amount.IsInt64() {
				telemetry.SetGaugeWithLabels(
					[]string{"tx", "msg", "create_permanent_locked_account"},
					float32(a.Amount.Int64()),
					[]metrics.Label{telemetry.NewLabel("denom", a.Denom)},
				)
			}
		}
	}()

	if err = s.SendCoins(ctx, from, to, msg.Amount); err != nil {
		return nil, err
	}

	return &types.MsgCreatePermanentLockedAccountResponse{}, nil
}

func (s msgServer) CreatePeriodicVestingAccount(goCtx context.Context, msg *types.MsgCreatePeriodicVestingAccount) (*types.MsgCreatePeriodicVestingAccountResponse, error) {
	from, err := s.AccountKeeper.AddressCodec().StringToBytes(msg.FromAddress)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid 'from' address: %s", err)
	}

	to, err := s.AccountKeeper.AddressCodec().StringToBytes(msg.ToAddress)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid 'to' address: %s", err)
	}

	if msg.StartTime < 1 {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid start time of %d, length must be greater than 0", msg.StartTime)
	}

	var totalCoins sdk.Coins
	for i, period := range msg.VestingPeriods {
		if period.Length < 1 {
			return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid period length of %d in period %d, length must be greater than 0", period.Length, i)
		}

		if err := validateAmount(period.Amount); err != nil {
			return nil, err
		}

		totalCoins = totalCoins.Add(period.Amount...)
	}

	if s.BlockedAddr(to) {
		return nil, errorsmod.Wrapf(sdkerrors.ErrUnauthorized, "%s is not allowed to receive funds", msg.ToAddress)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	if acc := s.GetAccount(ctx, to); acc != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "account %s already exists", msg.ToAddress)
	}

	if err := s.IsSendEnabledCoins(ctx, totalCoins...); err != nil {
		return nil, err
	}

	baseAccount := authtypes.NewBaseAccountWithAddress(to)
	baseAccount = s.NewAccount(ctx, baseAccount).(*authtypes.BaseAccount)
	vestingAccount, err := types.NewPeriodicVestingAccount(baseAccount, totalCoins.Sort(), msg.StartTime, msg.VestingPeriods)
	if err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, err.Error())
	}

	s.SetAccount(ctx, vestingAccount)

	defer func() {
		telemetry.IncrCounter(1, "new", "account")

		for _, a := range totalCoins {
			if a.Amount.IsInt64() {
				telemetry.SetGaugeWithLabels(
					[]string{"tx", "msg", "create_periodic_vesting_account"},
					float32(a.Amount.Int64()),
					[]metrics.Label{telemetry.NewLabel("denom", a.Denom)},
				)
			}
		}
	}()

	if err = s.SendCoins(ctx, from, to, totalCoins); err != nil {
		return nil, err
	}

	return &types.MsgCreatePeriodicVestingAccountResponse{}, nil
}

func validateAmount(amount sdk.Coins) error {
	if !amount.IsValid() {
		return sdkerrors.ErrInvalidCoins.Wrap(amount.String())
	}

	if !amount.IsAllPositive() {
		return sdkerrors.ErrInvalidCoins.Wrap(amount.String())
	}

	return nil
}

func (s msgServer) DonateAllVestingTokens(goCtx context.Context, msg *types.MsgDonateAllVestingTokens) (*types.MsgDonateAllVestingTokensResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	ak := s.AccountKeeper
	dk := s.DistrKeeper
	sk := s.StakingKeeper

	from, err := sdk.AccAddressFromBech32(msg.FromAddress)
	if err != nil {
		return nil, err
	}

	acc := ak.GetAccount(ctx, from)
	if acc == nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("account %s not exists", msg.FromAddress)
	}

	// get all delegations of an account and undust those that have less than 1 uluna
	delegations, err := sk.GetDelegatorDelegations(ctx, acc.GetAddress(), math.MaxUint16)
	if err != nil {
		return nil, err
	}
	for _, delegation := range delegations {
		validatorAddr, err := sdk.ValAddressFromBech32(delegation.ValidatorAddress)
		if err != nil {
			return nil, err
		}
		validator, err := sk.GetValidator(ctx, validatorAddr)
		if err != nil {
			return nil, err
		}
		// Try to delete the dust delegation
		_, removedTokens, err := sk.RemoveValidatorTokensAndShares(ctx, validator, delegation.Shares)
		if err != nil {
			return nil, err
		}
		// If the delegation is not dust, return an error and stop the donation flow
		if !removedTokens.IsZero() {
			return nil, sdkerrors.ErrInvalidRequest.Wrapf("account %s has a non-zero staking entry", msg.FromAddress)
		}

		// Remove the dust delegation shares from the validator
		err = sk.RemoveDelegation(ctx, delegation)
		if err != nil {
			return nil, err
		}
	}

	// check whether an account has any other type of staking entries
	unbondingDelegations, err := sk.GetUnbondingDelegations(ctx, acc.GetAddress(), math.MaxUint16)
	if err != nil {
		return nil, err
	}
	redelegations, err := sk.GetRedelegations(ctx, acc.GetAddress(), math.MaxUint16)
	if err != nil {
		return nil, err
	}
	if len(unbondingDelegations) != 0 ||
		len(redelegations) != 0 {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("account %s has staking entry", msg.FromAddress)
	}

	vestingAcc, ok := acc.(exported.VestingAccount)
	if !ok {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("account %s is not a vesting account", msg.FromAddress)
	}

	vestingCoins := vestingAcc.GetVestingCoins(ctx.BlockTime())
	if vestingCoins.IsZero() {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("account %s has no vesting tokens", msg.FromAddress)
	}

	// Change the account as normal account
	ak.SetAccount(ctx,
		authtypes.NewBaseAccount(
			acc.GetAddress(),
			acc.GetPubKey(),
			acc.GetAccountNumber(),
			acc.GetSequence(),
		),
	)

	if err := dk.FundCommunityPool(ctx, vestingCoins, from); err != nil {
		return nil, err
	}

	return &types.MsgDonateAllVestingTokensResponse{}, nil
}
