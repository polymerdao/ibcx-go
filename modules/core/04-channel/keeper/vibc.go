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
	// a virtual connection can ONLY be created with a Polymer client and a virtual client on the Polymer Chain
	return connection.ClientId == exported.PolymerClientID || connection.Counterparty.ClientId == exported.PolymerClientID, &connection
}
