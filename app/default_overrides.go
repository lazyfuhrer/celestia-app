package app

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/celestiaorg/celestia-app/pkg/appconsts"
	"github.com/celestiaorg/celestia-app/x/mint"
	minttypes "github.com/celestiaorg/celestia-app/x/mint/types"
	"github.com/cosmos/cosmos-sdk/codec"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/bank"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	distrclient "github.com/cosmos/cosmos-sdk/x/distribution/client"
	"github.com/cosmos/cosmos-sdk/x/gov"
	govclient "github.com/cosmos/cosmos-sdk/x/gov/client"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	paramsclient "github.com/cosmos/cosmos-sdk/x/params/client"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradeclient "github.com/cosmos/cosmos-sdk/x/upgrade/client"
	ibcclientclient "github.com/cosmos/ibc-go/v6/modules/core/02-client/client"
	tmcfg "github.com/tendermint/tendermint/config"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	coretypes "github.com/tendermint/tendermint/types"
)

// bankModule defines a custom wrapper around the x/bank module's AppModuleBasic
// implementation to provide custom default genesis state.
type bankModule struct {
	bank.AppModuleBasic
}

// DefaultGenesis returns custom x/bank module genesis state.
func (bankModule) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	metadata := banktypes.Metadata{
		Description: "The native staking token of the Celestia network.",
		Base:        BondDenom,
		Name:        DisplayDenom,
		Display:     DisplayDenom,
		Symbol:      DisplayDenom,
		DenomUnits: []*banktypes.DenomUnit{
			{
				Denom:    BondDenom,
				Exponent: 0,
				Aliases: []string{
					BondDenomAlias,
				},
			},
			{
				Denom:    DisplayDenom,
				Exponent: 6,
				Aliases:  []string{},
			},
		},
	}

	genState := banktypes.DefaultGenesisState()
	genState.DenomMetadata = append(genState.DenomMetadata, metadata)

	return cdc.MustMarshalJSON(genState)
}

// stakingModule wraps the x/staking module in order to overwrite specific
// ModuleManager APIs.
type stakingModule struct {
	staking.AppModuleBasic
}

// DefaultGenesis returns custom x/staking module genesis state.
func (stakingModule) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	params := stakingtypes.DefaultParams()
	params.UnbondingTime = appconsts.DefaultUnbondingTime
	params.BondDenom = BondDenom

	return cdc.MustMarshalJSON(&stakingtypes.GenesisState{
		Params: params,
	})
}

type crisisModule struct {
	crisis.AppModuleBasic
}

// DefaultGenesis returns custom x/crisis module genesis state.
func (crisisModule) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(&crisistypes.GenesisState{
		ConstantFee: sdk.NewCoin(BondDenom, sdk.NewInt(1000)),
	})
}

type mintModule struct {
	mint.AppModuleBasic
}

// DefaultGenesis returns custom x/mint module genesis state.
func (mintModule) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	genState := minttypes.DefaultGenesisState()
	genState.Minter.BondDenom = BondDenom

	return cdc.MustMarshalJSON(genState)
}

func newGovModule() govModule {
	return govModule{gov.NewAppModuleBasic(getLegacyProposalHandlers())}
}

// govModule is a custom wrapper around the x/gov module's AppModuleBasic
// implementation to provide custom default genesis state.
type govModule struct {
	gov.AppModuleBasic
}

// DefaultGenesis returns custom x/gov module genesis state.
func (govModule) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	genState := govtypes.DefaultGenesisState()
	genState.DepositParams.MinDeposit = sdk.NewCoins(sdk.NewCoin(BondDenom, sdk.NewInt(10000000)))

	return cdc.MustMarshalJSON(genState)
}

func getLegacyProposalHandlers() (result []govclient.ProposalHandler) {
	result = append(result,
		paramsclient.ProposalHandler,
		distrclient.ProposalHandler,
		upgradeclient.LegacyProposalHandler,
		upgradeclient.LegacyCancelProposalHandler,
		ibcclientclient.UpdateClientProposalHandler,
		ibcclientclient.UpgradeProposalHandler,
	)

	return result
}

// DefaultConsensusParams returns a ConsensusParams with a MaxBytes
// determined using a goal square size.
func DefaultConsensusParams() *tmproto.ConsensusParams {
	return &tmproto.ConsensusParams{
		Block:     DefaultBlockParams(),
		Evidence:  coretypes.DefaultEvidenceParams(),
		Validator: coretypes.DefaultValidatorParams(),
		Version: tmproto.VersionParams{
			AppVersion: appconsts.LatestVersion,
		},
	}
}

// DefaultBlockParams returns a default BlockParams with a MaxBytes determined
// using a goal square size.
func DefaultBlockParams() tmproto.BlockParams {
	return tmproto.BlockParams{
		MaxBytes:   appconsts.DefaultMaxBytes,
		MaxGas:     -1,
		TimeIotaMs: 1, // 1ms
	}
}

func DefaultConsensusConfig() *tmcfg.Config {
	cfg := tmcfg.DefaultConfig()
	// Set broadcast timeout to be 50 seconds in order to avoid timeouts for long block times
	// TODO: make TimeoutBroadcastTx configurable per https://github.com/celestiaorg/celestia-app/issues/1034
	cfg.RPC.TimeoutBroadcastTxCommit = 50 * time.Second
	cfg.RPC.MaxBodyBytes = int64(8388608) // 8 MiB
	cfg.Mempool.TTLNumBlocks = 10
	// Given that there is a stateful transaction size check in CheckTx,
	// We set a loose upper bound on what we expect the transaction to
	// be based on the upper bound size of the entire block for the given
	// version. This acts as a first line of DoS protection
	upperBoundBytes := appconsts.DefaultSquareSizeUpperBound * appconsts.DefaultSquareSizeUpperBound * appconsts.ContinuationSparseShareContentSize
	cfg.Mempool.MaxTxBytes = upperBoundBytes

	cfg.Mempool.Version = "v1" // prioritized mempool
	cfg.Consensus.TimeoutPropose = appconsts.TimeoutPropose
	cfg.Consensus.TimeoutCommit = appconsts.TimeoutCommit
	cfg.Consensus.SkipTimeoutCommit = false
	cfg.TxIndex.Indexer = "null"
	return cfg
}

func DefaultAppConfig() *serverconfig.Config {
	cfg := serverconfig.DefaultConfig()
	cfg.API.Enable = true

	// the default snapshot interval was determined by picking a large enough
	// value as to not dramatically increase resource requirements while also
	// being greater than zero so that there are more nodes that will serve
	// snapshots to nodes that state sync
	cfg.StateSync.SnapshotInterval = 1500
	cfg.StateSync.SnapshotKeepRecent = 2
	cfg.MinGasPrices = fmt.Sprintf("%v%s", appconsts.DefaultMinGasPrice, BondDenom)
	return cfg
}
