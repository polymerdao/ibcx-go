package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	connectiontypes "github.com/cosmos/ibc-go/v7/modules/core/03-connection/types"
	channeltypes "github.com/cosmos/ibc-go/v7/modules/core/04-channel/types"
)

/**
This files contains tx msg endpoint methods that override the default IBC behavior with virtual IBC checks in place
*/

// ChannelOpenInit defines a rpc handler method for MsgChannelOpenInit.
// ChannelOpenInit will perform 04-channel checks, route to the application
// callback, and write an OpenInit channel into state upon successful execution.
func (k Keeper) ChannelOpenInit(goCtx context.Context, msg *channeltypes.MsgChannelOpenInit) (*channeltypes.MsgChannelOpenInitResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// Ensure the first connection is not virtual; because ChannelOpenInit for virtual channel must go through
	// VIBC.OpenIBCChannel endpoint
	if isVirtual, connEnd := k.ChannelKeeper.IsVirtualConnection(ctx, msg.Channel.ConnectionHops[0]); isVirtual {
		return nil, sdkerrors.Wrapf(connectiontypes.ErrInvalidConnection, "ChanelOpenInit can only be invoked directly on a non-virtual connection, connection: %v", connEnd)
	}

	return k.ChannelOpenInitUnchecked(goCtx, msg)
}
