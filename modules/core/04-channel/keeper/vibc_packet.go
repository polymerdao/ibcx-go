package keeper

import (
	"strconv"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	capabilitytypes "github.com/cosmos/cosmos-sdk/x/capability/types"

	clienttypes "github.com/cosmos/ibc-go/v7/modules/core/02-client/types"
	connectiontypes "github.com/cosmos/ibc-go/v7/modules/core/03-connection/types"
	"github.com/cosmos/ibc-go/v7/modules/core/04-channel/types"
	host "github.com/cosmos/ibc-go/v7/modules/core/24-host"
	"github.com/cosmos/ibc-go/v7/modules/core/exported"
)

// SendVirtualPacket is called by a module in order to send a virtual IBC packet (ie. a packet from a virtual chain) on
// a channel.
//
// Unlike regular IBC packets, a virtual packet sequence was generated on the virtual chain instead of within core IBC module.
func (k Keeper) SendVirtualPacket(
	ctx sdk.Context,
	channelCap *capabilitytypes.Capability,
	sourcePort string,
	sourceChannel string,
	timeoutHeight clienttypes.Height,
	timeoutTimestamp uint64,
	data []byte,
	sequence uint64,
) (uint64, error) {
	channel, found := k.GetChannel(ctx, sourcePort, sourceChannel)
	if !found {
		return 0, sdkerrors.Wrap(types.ErrChannelNotFound, sourceChannel)
	}

	if channel.State != types.OPEN {
		return 0, sdkerrors.Wrapf(
			types.ErrInvalidChannelState,
			"channel is not OPEN (got %s)", channel.State.String(),
		)
	}

	if !k.scopedKeeper.AuthenticateCapability(ctx, channelCap, host.ChannelCapabilityPath(sourcePort, sourceChannel)) {
		return 0, sdkerrors.Wrapf(
			types.ErrChannelCapabilityNotFound,
			"caller does not own capability for channel, port ID (%s) channel ID (%s)",
			sourcePort,
			sourceChannel,
		)
	}

	// check if packet with same sequence already exists
	if k.HasPacketSendSeqProcessed(ctx, sourcePort, sourceChannel, sequence) {
		return 0, sdkerrors.Wrapf(
			types.ErrPacketAlreadyExists,
			"packet sequence (%d) already exists", sequence,
		)
	}

	// construct packet from given fields and channel state
	packet := types.NewPacket(data, sequence, sourcePort, sourceChannel,
		channel.Counterparty.PortId, channel.Counterparty.ChannelId, timeoutHeight, timeoutTimestamp)

	if err := packet.ValidateBasic(); err != nil {
		return 0, sdkerrors.Wrap(err, "constructed packet failed basic validation")
	}

	// Can not perform these extra checks when sending a multihop packet since the connectionEnd will not be known.
	if len(channel.ConnectionHops) == 1 {
		connectionEnd, found := k.connectionKeeper.GetConnection(ctx, channel.ConnectionHops[0])
		if !found {
			return 0, sdkerrors.Wrap(connectiontypes.ErrConnectionNotFound, channel.ConnectionHops[0])
		}

		clientState, found := k.clientKeeper.GetClientState(ctx, connectionEnd.GetClientID())
		if !found {
			return 0, clienttypes.ErrConsensusStateNotFound
		}

		// prevent accidental sends with clients that cannot be updated
		if status := k.clientKeeper.GetClientStatus(ctx, clientState, connectionEnd.GetClientID()); status != exported.Active {
			return 0, sdkerrors.Wrapf(clienttypes.ErrClientNotActive, "cannot send packet using client (%s) with status %s", connectionEnd.GetClientID(), status)
		}

		// check if packet is timed out on the receiving chain
		latestHeight := clientState.GetLatestHeight()
		if !timeoutHeight.IsZero() && latestHeight.GTE(timeoutHeight) {
			return 0, sdkerrors.Wrapf(
				types.ErrPacketTimeout,
				"receiving chain block height >= packet timeout height (%s >= %s)", latestHeight, timeoutHeight,
			)
		}

		latestTimestamp, err := k.connectionKeeper.GetTimestampAtHeight(ctx, connectionEnd, latestHeight)
		if err != nil {
			return 0, err
		}

		if packet.GetTimeoutTimestamp() != 0 && latestTimestamp >= packet.GetTimeoutTimestamp() {
			return 0, sdkerrors.Wrapf(
				types.ErrPacketTimeout,
				"receiving chain block timestamp >= packet timeout timestamp (%s >= %s)", time.Unix(0, int64(latestTimestamp)), time.Unix(0, int64(packet.GetTimeoutTimestamp())),
			)
		}
	}

	commitment := types.CommitPacket(k.cdc, packet)

	// k.SetNextSequenceSend(ctx, sourcePort, sourceChannel, sequence+1)
	k.SetPacketSendSeqProcessed(ctx, sourcePort, sourceChannel, sequence)
	k.SetPacketCommitment(ctx, sourcePort, sourceChannel, packet.GetSequence(), commitment)

	EmitSendPacketEvent(ctx, packet, channel, timeoutHeight)

	k.Logger(ctx).Info(
		"packet sent",
		"sequence", strconv.FormatUint(packet.GetSequence(), 10),
		"src_port", sourcePort,
		"src_channel", sourceChannel,
		"dst_port", packet.GetDestPort(),
		"dst_channel", packet.GetDestChannel(),
	)

	return packet.GetSequence(), nil
}
