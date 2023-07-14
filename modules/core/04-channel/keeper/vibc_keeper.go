package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	host "github.com/cosmos/ibc-go/v7/modules/core/24-host"
)

/**

This files contains virtual IBC specific logic for basic keeper CRUD operations.

Keeper logic that's regular IBC specific or shared by both virtual and regular IBC is located in keeper.go.

Do not add any virtual IBC specific logic unless it's absolutely necessary.

*/

// HasPacketSendSeqProcessed returns true if a packet sequence send has been processed.
// PacketCommitment can indicate the same, but only as long as the packet hasn't been acknowledged or timed out, by
// which point the PacketCommitment is deleted.
//
// This check prevents virtual packet replay attacks.
func (k Keeper) HasPacketSendSeqProcessed(ctx sdk.Context, port, channel string, sequence uint64) bool {
	store := ctx.KVStore(k.storeKey)
	key := host.NextSequenceSendProcessedKey(port, channel, sequence)
	return store.Has(key)
}

// SetPacketSendSeqProcessed sets the processed boolean for a packet sequence send.
// This check prevents virtual packet replay attacks.
func (k Keeper) SetPacketSendSeqProcessed(ctx sdk.Context, port, channel string, sequence uint64) {
	store := ctx.KVStore(k.storeKey)
	key := host.NextSequenceSendProcessedKey(port, channel, sequence)
	store.Set(key, []byte{byte(1)})
}
