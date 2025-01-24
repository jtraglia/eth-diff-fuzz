package main

import (
	"crypto/rand"

	"github.com/attestantio/go-eth2-client/spec/electra"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	gofuzzutils "github.com/trailofbits/go-fuzz-utils"
)

// generatePseudoRandomData generates pseudo-random data for testing.
func generatePseudoRandomData(shmMaxSize int) []byte {
	data := make([]byte, shmMaxSize)
	_, err := rand.Read(data)
	if err != nil {
		panic("Failed to generate random data")
	}
	return data
}

// getRandomBeaconState generates a pseudo-random beacon state.
func GetRandomBeaconState() (electra.BeaconState, error) {
	state := electra.BeaconState{}

	dataSize := 5 * 1024 * 1024
	data := generatePseudoRandomData(dataSize)
	tp, err := gofuzzutils.NewTypeProvider(data)
	if err != nil {
		return state, err
	}
	// No nil fields
	tp.SetParamsBiases(0, 0, 0, 0)
	if err != nil {
		return state, err
	}
	// Zero length slices, we'll fill these
	err = tp.SetParamsSliceBounds(0, 0)
	if err != nil {
		return state, err
	}

	err = tp.Fill(&state.GenesisTime)
	if err != nil {
		return state, err
	}
	err = tp.Fill(&state.GenesisValidatorsRoot)
	if err != nil {
		return state, err
	}
	err = tp.Fill(&state.Slot)
	if err != nil {
		return state, err
	}
	err = tp.Fill(&state.Fork)
	if err != nil {
		return state, err
	}
	err = tp.Fill(&state.LatestBlockHeader)
	if err != nil {
		return state, err
	}
	state.BlockRoots = make([]phase0.Root, 8192)
	for i := range 8192 {
		err = tp.Fill(&state.BlockRoots[i])
		if err != nil {
			return state, err
		}
	}
	state.StateRoots = make([]phase0.Root, 8192)
	for i := range 8192 {
		err = tp.Fill(&state.StateRoots[i])
		if err != nil {
			return state, err
		}
	}
	err = tp.Fill(&state.HistoricalRoots)
	if err != nil {
		return state, err
	}

	err = tp.Fill(&state.ETH1Data)
	if err != nil {
		return state, err
	}
	/* todo: blockhash really should be a [32]byte */
	state.ETH1Data.BlockHash, err = tp.GetNBytes(32)
	if err != nil {
		return state, err
	}

	numETH1DataVotes, err := tp.GetUint()
	// EPOCHS_PER_ETH1_VOTING_PERIOD * SLOTS_PER_EPOCH
	numETH1DataVotes %= 64 * 32
	state.ETH1DataVotes = make([]*phase0.ETH1Data, numETH1DataVotes)
	for i := range numETH1DataVotes {
		err = tp.Fill(&state.ETH1DataVotes[i])
		if err != nil {
			return state, err
		}
		state.ETH1DataVotes[i].BlockHash, err = tp.GetNBytes(32)
		if err != nil {
			return state, err
		}
	}

	err = tp.Fill(&state.ETH1DepositIndex)
	if err != nil {
		return state, err
	}

	numValidators, err := tp.GetUint()
	numValidators %= 10000 // 10k, arbitrary
	state.Validators = make([]*phase0.Validator, numValidators)
	state.Balances = make([]phase0.Gwei, numValidators)
	for i := range numValidators {
		err = tp.Fill(&state.Validators[i])
		if err != nil {
			return state, err
		}
		err = tp.Fill(&state.Balances[i])
		if err != nil {
			return state, err
		}

		// todo: make creds valid
		state.Validators[i].WithdrawalCredentials, err = tp.GetNBytes(32)
		if err != nil {
			return state, err
		}

		// todo: make balances valid
	}

	state.RANDAOMixes = make([]phase0.Root, 65536)
	for i := range 65536 {
		err = tp.Fill(&state.RANDAOMixes[i])
		if err != nil {
			return state, err
		}
	}

	state.Slashings = make([]phase0.Gwei, 8192)
	for i := range 8192 {
		err = tp.Fill(&state.RANDAOMixes[i])
		if err != nil {
			return state, err
		}
	}

	// previous_epoch_participation
	// current_epoch_participation

	state.JustificationBits = make([]byte, 1)
	for i := range 1 {
		err = tp.Fill(&state.JustificationBits[i])
		if err != nil {
			return state, err
		}
	}

	// previous_justified_checkpoint
	// current_justified_checkpoint
	// finalized_checkpoint
	// inactivity_scores

	err = tp.Fill(&state.CurrentSyncCommittee)
	if err != nil {
		return state, err
	}
	state.CurrentSyncCommittee.Pubkeys = make([]phase0.BLSPubKey, 512)
	for i := range 512 {
		err = tp.Fill(&state.CurrentSyncCommittee.Pubkeys[i])
		if err != nil {
			return state, err
		}
	}
	err = tp.Fill(&state.CurrentSyncCommittee.AggregatePubkey)
	if err != nil {
		return state, err
	}

	err = tp.Fill(&state.NextSyncCommittee)
	if err != nil {
		return state, err
	}
	state.NextSyncCommittee.Pubkeys = make([]phase0.BLSPubKey, 512)
	for i := range 512 {
		err = tp.Fill(&state.NextSyncCommittee.Pubkeys[i])
		if err != nil {
			return state, err
		}
	}
	err = tp.Fill(&state.NextSyncCommittee.AggregatePubkey)
	if err != nil {
		return state, err
	}

	err = tp.Fill(&state.LatestExecutionPayloadHeader)
	if err != nil {
		return state, err
	}

	// next_withdrawal_index
	// next_withdrawal_validator_index
	// historical_summaries
	// deposit_requests_start_index
	// deposit_balance_to_consume
	// exit_balance_to_consume
	// earliest_exit_epoch
	// consolidation_balance_to_consume
	// earliest_consolidation_epoch
	// pending_deposits
	// pending_partial_withdrawals
	// pending_consolidations

	return state, nil
}
