package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	connectiontypes "github.com/cosmos/ibc-go/v7/modules/core/03-connection/types"
	"github.com/cosmos/ibc-go/v7/modules/core/exported"
)

/**
This files contains virtual IBC related logic
*/

// IsVirtualConnection determines if the connection is virtual
func (k Keeper) IsVirtualConnection(ctx sdk.Context, connectionID string) (bool, *connectiontypes.ConnectionEnd) {
	connection, found := k.connectionKeeper.GetConnection(ctx, connectionID)
	if !found {
		return false, nil
	}
	return k.IsVirtualConnectionEnd(ctx, &connection), &connection
}

// IsVirtualConnectionEnd determines if the connection end is virtual
func (k Keeper) IsVirtualConnectionEnd(_ sdk.Context, connectionEnd *connectiontypes.ConnectionEnd) bool {
	// a virtual connection can ONLY be created with a Polymer client and a virtual client on the Polymer Chain
	return connectionEnd.ClientId == exported.PolymerClientID || connectionEnd.Counterparty.ClientId == exported.PolymerClientID
}

// Returns true in the vIBC -> vIBC scenario, i.e. both connection ends are virtual. Returns false otherwise
func (k Keeper) IsVirtualEndToVirtualEnd(ctx sdk.Context, connectionHops []string) bool {
	if len(connectionHops) != 2 {
		return false
	}
	hop0isVirtual, _ := k.IsVirtualConnection(ctx, connectionHops[0])
	hop1isVirtual, _ := k.IsVirtualConnection(ctx, connectionHops[1])
	return hop0isVirtual && hop1isVirtual
}
