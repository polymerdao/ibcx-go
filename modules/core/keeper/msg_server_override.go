package keeper

import (
	"context"
	"fmt"

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
func (k Keeper) ChannelOpenInit(
	goCtx context.Context,
	msg *channeltypes.MsgChannelOpenInit,
) (*channeltypes.MsgChannelOpenInitResponse, error) {
	err := k.ensureNonVirtualSender(goCtx, msg.Channel, "ChannelOpenInit")
	if err != nil {
		return nil, err
	}
	return k.ChannelOpenInitUnchecked(goCtx, msg)
}

func (k Keeper) ChannelOpenTry(goCtx context.Context, msg *channeltypes.MsgChannelOpenTry) (*channeltypes.MsgChannelOpenTryResponse, error) {
	err := k.ensureNonVirtualConnectionsForChannel(goCtx, "ChannelOpenTry", msg.Channel)
	if err != nil {
		return nil, err
	}
	return k.ChannelOpenTryUnchecked(goCtx, msg)
}

func (k Keeper) ChannelOpenAck(goCtx context.Context, msg *channeltypes.MsgChannelOpenAck) (*channeltypes.MsgChannelOpenAckResponse, error) {
	err := k.ensureNonVirtualConnections(goCtx, "ChannelOpenAck", msg.PortId, msg.ChannelId)
	if err != nil {
		return nil, err
	}
	return k.ChannelOpenAckUnchecked(goCtx, msg)
}

func (k Keeper) ChannelOpenConfirm(goCtx context.Context, msg *channeltypes.MsgChannelOpenConfirm) (*channeltypes.MsgChannelOpenConfirmResponse, error) {
	err := k.ensureNonVirtualConnections(goCtx, "ChannelOpenConfirm", msg.PortId, msg.ChannelId)
	if err != nil {
		return nil, err
	}
	return k.ChannelOpenConfirmUnchecked(goCtx, msg)
}

func (k Keeper) ChannelCloseInit(goCtx context.Context, msg *channeltypes.MsgChannelCloseInit) (*channeltypes.MsgChannelCloseInitResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	channel, found := k.ChannelKeeper.GetChannel(ctx, msg.PortId, msg.ChannelId)
	if !found {
		return nil, sdkerrors.Wrapf(channeltypes.ErrChannelNotFound, "port ID (%s) channel ID (%s)", msg.PortId, msg.ChannelId)
	}

	err := k.ensureNonVirtualSender(goCtx, channel, "ChannelCloseInit")
	if err != nil {
		return nil, err
	}

	return k.ChannelCloseInitUnchecked(goCtx, msg)
}

func (k Keeper) ChannelCloseConfirm(goCtx context.Context, msg *channeltypes.MsgChannelCloseConfirm) (*channeltypes.MsgChannelCloseConfirmResponse, error) {
	err := k.ensureNonVirtualConnections(goCtx, "ChannelCloseConfirm", msg.PortId, msg.ChannelId)
	if err != nil {
		return nil, err
	}
	return k.ChannelCloseConfirmUnchecked(goCtx, msg)
}

func (k Keeper) ChannelCloseFrozen(goCtx context.Context, msg *channeltypes.MsgChannelCloseFrozen) (*channeltypes.MsgChannelCloseFrozenResponse, error) {
	err := k.ensureNonVirtualConnections(goCtx, "ChannelCloseFrozen", msg.PortId, msg.ChannelId)
	if err != nil {
		return nil, err
	}
	return k.ChannelCloseFrozenUnchecked(goCtx, msg)
}

func (k Keeper) RecvPacket(goCtx context.Context, msg *channeltypes.MsgRecvPacket) (*channeltypes.MsgRecvPacketResponse, error) {
	err := k.ensureNonVirtualConnections(goCtx, "RecvPacket", msg.Packet.GetDestPort(), msg.Packet.GetDestChannel())
	if err != nil {
		return nil, err
	}
	return k.RecvPacketUnchecked(goCtx, msg)
}

func (k Keeper) Acknowledgement(goCtx context.Context, msg *channeltypes.MsgAcknowledgement) (*channeltypes.MsgAcknowledgementResponse, error) {
	err := k.ensureNonVirtualConnections(goCtx, "Acknowledgement", msg.Packet.GetSourcePort(), msg.Packet.GetSourceChannel())
	if err != nil {
		return nil, err
	}
	return k.AcknowledgementUnchecked(goCtx, msg)
}

func (k Keeper) ensureNonVirtualSender(goCtx context.Context, channel channeltypes.Channel, methodName string) error {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if isVirtual, connEnd := k.ChannelKeeper.IsVirtualConnection(ctx, channel.ConnectionHops[0]); isVirtual {
		return sdkerrors.Wrapf(
			connectiontypes.ErrInvalidConnection,
			"%s can only be invoked directly on a non-virtual connection, connection: %v",
			methodName, connEnd,
		)
	}
	return nil
}

func (k Keeper) ensureNonVirtualConnections(goCtx context.Context, methodName, portID, channelID string) error {
	ctx := sdk.UnwrapSDKContext(goCtx)
	channel, found := k.ChannelKeeper.GetChannel(ctx, portID, channelID)
	if !found {
		return sdkerrors.Wrapf(channeltypes.ErrChannelNotFound, "port ID (%s) channel ID (%s)", portID, channelID)
	}
	return k.ensureNonVirtualConnectionsForChannel(goCtx, methodName, channel)
}

func (k Keeper) ensureNonVirtualConnectionsForChannel(goCtx context.Context, methodName string, channel channeltypes.Channel) error {
	ctx := sdk.UnwrapSDKContext(goCtx)
	isVirtual := k.ChannelKeeper.IsVirtualEndToVirtualEnd(ctx, channel.ConnectionHops)
	if isVirtual {
		return sdkerrors.Wrapf(
			connectiontypes.ErrInvalidConnection,
			fmt.Sprintf("%s can only be invoked directly on non-virtual connections", methodName),
		)
	}
	return nil
}
