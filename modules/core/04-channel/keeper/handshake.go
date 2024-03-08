package keeper

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	capabilitytypes "github.com/cosmos/cosmos-sdk/x/capability/types"

	clienttypes "github.com/cosmos/ibc-go/v7/modules/core/02-client/types"
	connectiontypes "github.com/cosmos/ibc-go/v7/modules/core/03-connection/types"
	"github.com/cosmos/ibc-go/v7/modules/core/04-channel/types"
	porttypes "github.com/cosmos/ibc-go/v7/modules/core/05-port/types"
	host "github.com/cosmos/ibc-go/v7/modules/core/24-host"
	"github.com/cosmos/ibc-go/v7/modules/core/exported"
	tmclient "github.com/cosmos/ibc-go/v7/modules/light-clients/07-tendermint"
)

// ChanOpenInit is called by a module to initiate a channel opening handshake with
// a module on another chain. The counterparty channel identifier is validated to be
// empty in msg validation.
func (k Keeper) ChanOpenInit(
	ctx sdk.Context,
	order types.Order,
	connectionHops []string,
	portID string,
	portCap *capabilitytypes.Capability,
	counterparty types.Counterparty,
	version string,
) (string, *capabilitytypes.Capability, error) {
	// connection hop length checked on msg.ValidateBasic()
	connectionEnd, found := k.connectionKeeper.GetConnection(ctx, connectionHops[0])
	if !found {
		return "", nil, sdkerrors.Wrap(connectiontypes.ErrConnectionNotFound, connectionHops[0])
	}

	// ******************************************************************************************
	// TODO: This is a bug for a multihop channels. For multihop we need the connectionEnd
	// corresponding to the connection to chain Z for the logic to be meaningful.
	// ******************************************************************************************

	if len(connectionHops) == 1 {
		getVersions := connectionEnd.GetVersions()
		if len(getVersions) != 1 {
			return "", nil, sdkerrors.Wrapf(
				connectiontypes.ErrInvalidVersion,
				"single version must be negotiated on connection before opening channel, got: %v",
				getVersions,
			)
		}

		if !connectiontypes.VerifySupportedFeature(getVersions[0], order.String()) {
			return "", nil, sdkerrors.Wrapf(
				connectiontypes.ErrInvalidVersion,
				"connection version %s does not support channel ordering: %s",
				getVersions[0], order.String(),
			)
		}
	}

	clientState, found := k.clientKeeper.GetClientState(ctx, connectionEnd.ClientId)
	if !found {
		return "", nil, sdkerrors.Wrapf(clienttypes.ErrClientNotFound, "clientID (%s)", connectionEnd.ClientId)
	}

	if status := k.clientKeeper.GetClientStatus(ctx, clientState, connectionEnd.ClientId); status != exported.Active {
		return "", nil, sdkerrors.Wrapf(clienttypes.ErrClientNotActive, "client (%s) status is %s", connectionEnd.ClientId, status)
	}

	if !k.portKeeper.Authenticate(ctx, portCap, portID) {
		return "", nil, sdkerrors.Wrapf(
			porttypes.ErrInvalidPort,
			"caller does not own port capability for port ID %s",
			portID,
		)
	}

	channelID := k.GenerateChannelIdentifier(ctx)

	capKey, err := k.scopedKeeper.NewCapability(ctx, host.ChannelCapabilityPath(portID, channelID))
	if err != nil {
		return "", nil, sdkerrors.Wrapf(
			err,
			"could not create channel capability for port ID %s and channel ID %s",
			portID,
			channelID,
		)
	}

	return channelID, capKey, nil
}

// WriteOpenInitChannel writes a channel which has successfully passed the OpenInit handshake step.
// The channel is set in state and all the associated Send and Recv sequences are set to 1.
// An event is emitted for the handshake step.
func (k Keeper) WriteOpenInitChannel(
	ctx sdk.Context,
	portID,
	channelID string,
	order types.Order,
	connectionHops []string,
	counterparty types.Counterparty,
	version string,
) {
	channel := types.NewChannel(types.INIT, order, counterparty, connectionHops, version)
	k.SetChannel(ctx, portID, channelID, channel)

	k.SetNextSequenceSend(ctx, portID, channelID, 1)
	k.SetNextSequenceRecv(ctx, portID, channelID, 1)
	k.SetNextSequenceAck(ctx, portID, channelID, 1)

	k.Logger(ctx).
		Info("channel state updated", "port-id", portID, "channel-id", channelID, "previous-state", "NONE", "new-state", "INIT")

	defer func() {
		telemetry.IncrCounter(1, "ibc", "channel", "open-init")
	}()

	EmitChannelOpenInitEvent(ctx, portID, channelID, channel)
}

// ChanOpenTry is called by a module to accept the first step of a channel opening
// handshake initiated by a module on another chain.
func (k Keeper) ChanOpenTry(
	ctx sdk.Context,
	order types.Order,
	connectionHops []string,
	portID string,
	portCap *capabilitytypes.Capability,
	counterparty types.Counterparty,
	counterpartyVersion string,
	proofInit []byte,
	proofHeight exported.Height,
) (string, *capabilitytypes.Capability, error) {

	// generate a new channel
	channelID := k.GenerateChannelIdentifier(ctx)

	if !k.portKeeper.Authenticate(ctx, portCap, portID) {
		return "", nil, sdkerrors.Wrapf(
			porttypes.ErrInvalidPort,
			"caller does not own port capability for port ID %s",
			portID,
		)
	}

	// Directly verify the last connectionHop. In a multihop hop scenario only the final
	// connection hop can be verified directly. The remaining connections are verified below.
	connectionEnd, found := k.connectionKeeper.GetConnection(ctx, connectionHops[0])
	if !found {
		return "", nil, sdkerrors.Wrap(connectiontypes.ErrConnectionNotFound, connectionHops[0])
	}

	if connectionEnd.GetState() != int32(connectiontypes.OPEN) {
		return "", nil, sdkerrors.Wrapf(
			connectiontypes.ErrInvalidConnectionState,
			"connection state is not OPEN (got %s)", connectiontypes.State(connectionEnd.GetState()).String(),
		)
	}

	if err := k.verifyChanOpenTryProof(ctx, order, connectionHops, portID, counterparty, counterpartyVersion, proofInit, proofHeight, &connectionEnd); err != nil {
		return "", nil, err
	}

	var (
		capKey *capabilitytypes.Capability
		err    error
	)

	capKey, err = k.scopedKeeper.NewCapability(ctx, host.ChannelCapabilityPath(portID, channelID))
	if err != nil {
		return "", nil, sdkerrors.Wrapf(
			err,
			"could not create channel capability for port ID %s and channel ID %s",
			portID,
			channelID,
		)
	}

	return channelID, capKey, nil
}

func (k Keeper) verifyChanOpenTryProof(
	ctx sdk.Context,
	order types.Order,
	connectionHops []string,
	portID string,
	counterparty types.Counterparty,
	counterpartyVersion string,
	proofInit []byte,
	proofHeight exported.Height,
	connectionEnd *connectiontypes.ConnectionEnd,
) error {
	// in the virtual to virtual case we don't need to check for proofs so the rest of the code is by-passed
	if k.IsVirtualEndToVirtualEnd(ctx, connectionHops) {
		return nil
	}

	// check version support
	versionCheckFunc := func(connection *connectiontypes.ConnectionEnd) error {
		getVersions := connection.GetVersions()
		if len(getVersions) != 1 {
			return sdkerrors.Wrapf(
				connectiontypes.ErrInvalidVersion,
				"single version must be negotiated on connection before opening channel, got: %v",
				getVersions,
			)
		}

		// verify chain A supports the requested ordering.
		if !connectiontypes.VerifySupportedFeature(getVersions[0], order.String()) {
			return sdkerrors.Wrapf(
				connectiontypes.ErrInvalidVersion,
				"connection version %s does not support channel ordering: %s",
				getVersions[0], order.String(),
			)
		}
		return nil
	}
	// handle multihop case
	if len(connectionHops) > 1 {
		kvGenerator := func(mProof *types.MsgMultihopProofs, multihopConnectionEnd *connectiontypes.ConnectionEnd) (string, []byte, error) {
			// check version support
			if err := versionCheckFunc(multihopConnectionEnd); err != nil {
				return "", nil, err
			}

			key := host.ChannelPath(counterparty.PortId, counterparty.ChannelId)

			counterpartyHops, err := mProof.GetCounterpartyHops(k.cdc, connectionEnd)
			if err != nil {
				return "", nil, err
			}
			// expectedCounterparty is the counterparty of the counterparty's channel end
			// (i.e self)
			expectedCounterparty := types.NewCounterparty(portID, "")
			expectedChannel := types.NewChannel(
				types.INIT, order, expectedCounterparty,
				counterpartyHops, counterpartyVersion,
			)

			// expected value bytes
			value, err := expectedChannel.Marshal()
			if err != nil {
				return "", nil, err
			}

			return key, value, nil
		}

		if err := k.connectionKeeper.VerifyMultihopMembership(
			ctx, connectionEnd, proofHeight, proofInit,
			connectionHops, kvGenerator); err != nil {
			return err
		}

	} else {
		// determine counterparty hops
		counterpartyHops := []string{connectionEnd.GetCounterparty().GetConnectionID()}

		// check versions
		if err := versionCheckFunc(connectionEnd); err != nil {
			return err
		}

		// expectedCounterparty is the counterparty of the counterparty's channel end
		// (i.e self)
		expectedCounterparty := types.NewCounterparty(portID, "")
		expectedChannel := types.NewChannel(
			types.INIT, order, expectedCounterparty,
			counterpartyHops, counterpartyVersion,
		)

		if err := k.connectionKeeper.VerifyChannelState(
			ctx, connectionEnd, proofHeight, proofInit,
			counterparty.PortId, counterparty.ChannelId, expectedChannel,
		); err != nil {
			return err
		}
	}
	return nil
}

// WriteOpenTryChannel writes a channel which has successfully passed the OpenTry handshake step.
// The channel is set in state. If a previous channel state did not exist, all the Send and Recv
// sequences are set to 1. An event is emitted for the handshake step.
func (k Keeper) WriteOpenTryChannel(
	ctx sdk.Context,
	portID,
	channelID string,
	order types.Order,
	connectionHops []string,
	counterparty types.Counterparty,
	version string,
) {
	k.SetNextSequenceSend(ctx, portID, channelID, 1)
	k.SetNextSequenceRecv(ctx, portID, channelID, 1)
	k.SetNextSequenceAck(ctx, portID, channelID, 1)

	// set channel state to TRY_PENDING if the first connection hop is virtual; otherwise, set it to TRYOPEN
	isVirtual, _ := k.IsVirtualConnection(ctx, connectionHops[0])
	var channelState types.State = types.TRYOPEN
	if isVirtual {
		channelState = types.TRY_PENDING
	}

	channel := types.NewChannel(channelState, order, counterparty, connectionHops, version)

	k.SetChannel(ctx, portID, channelID, channel)

	k.Logger(ctx).
		Info("channel state updated", "port-id", portID, "channel-id", channelID, "previous-state", "NONE", "new-state", "TRYOPEN")

	defer func() {
		telemetry.IncrCounter(1, "ibc", "channel", "open-try")
	}()

	EmitChannelOpenTryEvent(ctx, portID, channelID, channel)
}

// ChanOpenAck is called by the handshake-originating module to acknowledge the
// acceptance of the initial request by the counterparty module on the other chain.
func (k Keeper) ChanOpenAck(
	ctx sdk.Context,
	portID,
	channelID string,
	chanCap *capabilitytypes.Capability,
	counterpartyVersion,
	counterpartyChannelID string,
	proofTry []byte,
	proofHeight exported.Height,
) error {
	channel, found := k.GetChannel(ctx, portID, channelID)
	if !found {
		return sdkerrors.Wrapf(types.ErrChannelNotFound, "port ID (%s) channel ID (%s)", portID, channelID)
	}

	if channel.State != types.INIT {
		return sdkerrors.Wrapf(
			types.ErrInvalidChannelState,
			"channel state should be INIT (got %s)",
			channel.State.String(),
		)
	}

	if !k.scopedKeeper.AuthenticateCapability(ctx, chanCap, host.ChannelCapabilityPath(portID, channelID)) {
		return sdkerrors.Wrapf(
			types.ErrChannelCapabilityNotFound,
			"caller does not own capability for channel, port ID (%s) channel ID (%s)",
			portID,
			channelID,
		)
	}

	connectionEnd, found := k.connectionKeeper.GetConnection(ctx, channel.ConnectionHops[0])
	if !found {
		return sdkerrors.Wrap(connectiontypes.ErrConnectionNotFound, channel.ConnectionHops[0])
	}

	if connectionEnd.GetState() != int32(connectiontypes.OPEN) {
		return sdkerrors.Wrapf(
			connectiontypes.ErrInvalidConnectionState,
			"connection state is not OPEN (got %s)", connectiontypes.State(connectionEnd.GetState()).String(),
		)
	}

	// in the virtual to virtual case we don't need to check for proofs so the rest of the code is by-passed
	if k.IsVirtualEndToVirtualEnd(ctx, channel.ConnectionHops) {
		return nil
	}

	// verify multihop proof
	if len(channel.ConnectionHops) > 1 {

		kvGenerator := func(mProof *types.MsgMultihopProofs, _ *connectiontypes.ConnectionEnd) (string, []byte, error) {
			key := host.ChannelPath(channel.Counterparty.PortId, counterpartyChannelID)

			counterpartyHops, err := mProof.GetCounterpartyHops(k.cdc, &connectionEnd)
			if err != nil {
				return "", nil, err
			}
			expectedCounterparty := types.NewCounterparty(portID, channelID)
			expectedChannel := types.NewChannel(
				types.TRYOPEN, channel.Ordering, expectedCounterparty,
				counterpartyHops, counterpartyVersion,
			)
			value, err := expectedChannel.Marshal()
			if err != nil {
				return "", nil, err
			}
			return key, value, nil
		}

		if err := k.connectionKeeper.VerifyMultihopMembership(
			ctx, connectionEnd, proofHeight, proofTry,
			channel.ConnectionHops, kvGenerator); err != nil {
			return err
		}

	} else {
		counterpartyHops := []string{connectionEnd.GetCounterparty().GetConnectionID()}
		expectedCounterparty := types.NewCounterparty(portID, channelID)
		expectedChannel := types.NewChannel(
			types.TRYOPEN, channel.Ordering, expectedCounterparty,
			counterpartyHops, counterpartyVersion,
		)
		if err := k.connectionKeeper.VerifyChannelState(
			ctx, connectionEnd, proofHeight, proofTry,
			channel.Counterparty.PortId, counterpartyChannelID, expectedChannel,
		); err != nil {
			return err
		}
	}

	return nil
}

// WriteOpenAckChannel writes an updated channel state for the successful OpenAck handshake step.
// An event is emitted for the handshake step.
func (k Keeper) WriteOpenAckChannel(
	ctx sdk.Context,
	portID,
	channelID,
	counterpartyVersion,
	counterpartyChannelID string,
) {
	channel, found := k.GetChannel(ctx, portID, channelID)
	if !found {
		panic(
			fmt.Sprintf(
				"could not find existing channel when updating channel state in successful ChanOpenAck step, channelID: %s, portID: %s",
				channelID,
				portID,
			),
		)
	}

	// set channel state to ACK_PENDING if the first connection is virtual; otherwise, set it to OPEN
	isVirtual, _ := k.IsVirtualConnection(ctx, channel.ConnectionHops[0])
	var channelState types.State = types.OPEN
	if isVirtual {
		channelState = types.ACK_PENDING
	}

	channel.State = channelState
	channel.Version = counterpartyVersion
	channel.Counterparty.ChannelId = counterpartyChannelID
	k.SetChannel(ctx, portID, channelID, channel)

	k.Logger(ctx).
		Info("channel state updated", "port-id", portID, "channel-id", channelID, "previous-state", channel.State.String(), "new-state", "OPEN")

	defer func() {
		telemetry.IncrCounter(1, "ibc", "channel", "open-ack")
	}()

	EmitChannelOpenAckEvent(ctx, portID, channelID, channel)
}

// ChanOpenConfirm is called by the counterparty module to close their end of the
// channel, since the other end has been closed.
func (k Keeper) ChanOpenConfirm(
	ctx sdk.Context,
	portID,
	channelID string,
	chanCap *capabilitytypes.Capability,
	proofAck []byte,
	proofHeight exported.Height,
) error {
	channel, found := k.GetChannel(ctx, portID, channelID)
	if !found {
		return sdkerrors.Wrapf(types.ErrChannelNotFound, "port ID (%s) channel ID (%s)", portID, channelID)
	}

	if channel.State != types.TRYOPEN {
		return sdkerrors.Wrapf(
			types.ErrInvalidChannelState,
			"channel state is not TRYOPEN (got %s)", channel.State.String(),
		)
	}

	if !k.scopedKeeper.AuthenticateCapability(ctx, chanCap, host.ChannelCapabilityPath(portID, channelID)) {
		return sdkerrors.Wrapf(
			types.ErrChannelCapabilityNotFound,
			"caller does not own capability for channel, port ID (%s) channel ID (%s)",
			portID,
			channelID,
		)
	}

	connectionEnd, found := k.connectionKeeper.GetConnection(ctx, channel.ConnectionHops[0])
	if !found {
		return sdkerrors.Wrap(connectiontypes.ErrConnectionNotFound, channel.ConnectionHops[0])
	}

	if connectionEnd.GetState() != int32(connectiontypes.OPEN) {
		return sdkerrors.Wrapf(
			connectiontypes.ErrInvalidConnectionState,
			"connection state is not OPEN (got %s)", connectiontypes.State(connectionEnd.GetState()).String(),
		)
	}

	// in the virtual to virtual case we don't need to check for proofs so the rest of the code is by-passed
	if k.IsVirtualEndToVirtualEnd(ctx, channel.ConnectionHops) {
		return nil
	}

	// verify multihop proof or standard proof
	if len(channel.ConnectionHops) > 1 {
		kvGenerator := func(mProof *types.MsgMultihopProofs, _ *connectiontypes.ConnectionEnd) (string, []byte, error) {
			key := host.ChannelPath(channel.Counterparty.PortId, channel.Counterparty.ChannelId)

			counterpartyHops, err := mProof.GetCounterpartyHops(k.cdc, &connectionEnd)
			if err != nil {
				return "", nil, err
			}
			counterparty := types.NewCounterparty(portID, channelID)
			expectedChannel := types.NewChannel(
				types.OPEN, channel.Ordering, counterparty,
				counterpartyHops, channel.Version,
			)
			value, err := expectedChannel.Marshal()
			if err != nil {
				return "", nil, err
			}
			return key, value, nil
		}

		if err := k.connectionKeeper.VerifyMultihopMembership(
			ctx, connectionEnd, proofHeight, proofAck,
			channel.ConnectionHops, kvGenerator); err != nil {
			return err
		}

	} else {
		counterpartyHops := []string{connectionEnd.GetCounterparty().GetConnectionID()}
		counterparty := types.NewCounterparty(portID, channelID)
		expectedChannel := types.NewChannel(
			types.OPEN, channel.Ordering, counterparty,
			counterpartyHops, channel.Version,
		)
		if err := k.connectionKeeper.VerifyChannelState(
			ctx, connectionEnd, proofHeight, proofAck,
			channel.Counterparty.PortId, channel.Counterparty.ChannelId,
			expectedChannel,
		); err != nil {
			return err
		}
	}

	return nil
}

// WriteOpenConfirmChannel writes an updated channel state for the successful OpenConfirm handshake step.
// An event is emitted for the handshake step.
func (k Keeper) WriteOpenConfirmChannel(
	ctx sdk.Context,
	portID,
	channelID string,
) {
	channel, found := k.GetChannel(ctx, portID, channelID)
	if !found {
		panic(
			fmt.Sprintf(
				"could not find existing channel when updating channel state in successful ChanOpenConfirm step, channelID: %s, portID: %s",
				channelID,
				portID,
			),
		)
	}

	// set channel state to CONFIRM_PENDING if the first connection is virtual; otherwise, set it to OPEN
	isVirtual, _ := k.IsVirtualConnection(ctx, channel.ConnectionHops[0])
	var channelState types.State = types.OPEN
	if isVirtual {
		channelState = types.CONFIRM_PENDING
	}

	channel.State = channelState
	k.SetChannel(ctx, portID, channelID, channel)
	k.Logger(ctx).
		Info("channel state updated", "port-id", portID, "channel-id", channelID, "previous-state", "TRYOPEN", "new-state", "OPEN")

	defer func() {
		telemetry.IncrCounter(1, "ibc", "channel", "open-confirm")
	}()

	EmitChannelOpenConfirmEvent(ctx, portID, channelID, channel)
}

// Closing Handshake
//
// This section defines the set of functions required to close a channel handshake
// as defined in https://github.com/cosmos/ibc/tree/master/spec/core/ics-004-channel-and-packet-semantics#closing-handshake
//
// ChanCloseInit is called by either module to close their end of the channel. Once
// closed, channels cannot be reopened.
func (k Keeper) ChanCloseInit(
	ctx sdk.Context,
	portID,
	channelID string,
	chanCap *capabilitytypes.Capability,
) error {
	if !k.scopedKeeper.AuthenticateCapability(ctx, chanCap, host.ChannelCapabilityPath(portID, channelID)) {
		return sdkerrors.Wrapf(
			types.ErrChannelCapabilityNotFound,
			"caller does not own capability for channel, port ID (%s) channel ID (%s)",
			portID,
			channelID,
		)
	}

	channel, found := k.GetChannel(ctx, portID, channelID)
	if !found {
		return sdkerrors.Wrapf(types.ErrChannelNotFound, "port ID (%s) channel ID (%s)", portID, channelID)
	}

	if channel.State == types.CLOSED {
		return sdkerrors.Wrap(types.ErrInvalidChannelState, "channel is already CLOSED")
	}

	// TODO: skip this if connectionHops > 1 (alternative would be to pass connection state along with proof)

	connectionEnd, found := k.connectionKeeper.GetConnection(ctx, channel.ConnectionHops[0])
	if !found {
		return sdkerrors.Wrap(connectiontypes.ErrConnectionNotFound, channel.ConnectionHops[0])
	}

	clientState, found := k.clientKeeper.GetClientState(ctx, connectionEnd.ClientId)
	if !found {
		return sdkerrors.Wrapf(clienttypes.ErrClientNotFound, "clientID (%s)", connectionEnd.ClientId)
	}

	if status := k.clientKeeper.GetClientStatus(ctx, clientState, connectionEnd.ClientId); status != exported.Active {
		return sdkerrors.Wrapf(clienttypes.ErrClientNotActive, "client (%s) status is %s", connectionEnd.ClientId, status)
	}

	if connectionEnd.GetState() != int32(connectiontypes.OPEN) {
		return sdkerrors.Wrapf(
			connectiontypes.ErrInvalidConnectionState,
			"connection state is not OPEN (got %s)", connectiontypes.State(connectionEnd.GetState()).String(),
		)
	}

	k.Logger(ctx).
		Info("channel state updated", "port-id", portID, "channel-id", channelID, "previous-state", channel.State.String(), "new-state", "CLOSED")

	defer func() {
		telemetry.IncrCounter(1, "ibc", "channel", "close-init")
	}()

	channel.State = types.CLOSED
	k.SetChannel(ctx, portID, channelID, channel)

	EmitChannelCloseInitEvent(ctx, portID, channelID, channel)

	return nil
}

// ChanCloseConfirm is called by the counterparty module to close their end of the
// channel, since the other end has been closed.
func (k Keeper) ChanCloseConfirm(
	ctx sdk.Context,
	portID,
	channelID string,
	chanCap *capabilitytypes.Capability,
	proofInit []byte,
	proofHeight exported.Height,
) error {
	if !k.scopedKeeper.AuthenticateCapability(ctx, chanCap, host.ChannelCapabilityPath(portID, channelID)) {
		return sdkerrors.Wrap(
			types.ErrChannelCapabilityNotFound,
			"caller does not own capability for channel, port ID (%s) channel ID (%s)",
		)
	}

	channel, found := k.GetChannel(ctx, portID, channelID)
	if !found {
		return sdkerrors.Wrapf(types.ErrChannelNotFound, "port ID (%s) channel ID (%s)", portID, channelID)
	}

	if channel.State == types.CLOSED || channel.State == types.CLOSE_CONFIRM_PENDING {
		var sState string
		if channel.State == types.CLOSED {
			sState = "CLOSED"
		} else {
			sState = "CLOSE_CONFIRM_PENDING"
		}

		return sdkerrors.Wrapf(types.ErrInvalidChannelState, "channel is already %s", sState)
	}

	connectionEnd, found := k.connectionKeeper.GetConnection(ctx, channel.ConnectionHops[0])
	if !found {
		return sdkerrors.Wrap(
			connectiontypes.ErrConnectionNotFound,
			channel.ConnectionHops[0],
		)
	}

	if connectionEnd.GetState() != int32(connectiontypes.OPEN) {
		return sdkerrors.Wrapf(
			connectiontypes.ErrInvalidConnectionState,
			"connection state is not OPEN (got %s)", connectiontypes.State(connectionEnd.GetState()).String(),
		)
	}

	// verify multihop proof
	if len(channel.ConnectionHops) > 1 {

		kvGenerator := func(mProof *types.MsgMultihopProofs, _ *connectiontypes.ConnectionEnd) (string, []byte, error) {
			key := host.ChannelPath(channel.Counterparty.PortId, channel.Counterparty.ChannelId)

			counterpartyHops, err := mProof.GetCounterpartyHops(k.cdc, &connectionEnd)
			if err != nil {
				return "", nil, err
			}
			counterparty := types.NewCounterparty(portID, channelID)
			expectedChannel := types.NewChannel(
				types.CLOSED, channel.Ordering, counterparty,
				counterpartyHops, channel.Version,
			)
			value, err := expectedChannel.Marshal()
			if err != nil {
				return "", nil, err
			}
			return key, value, nil
		}

		if err := k.connectionKeeper.VerifyMultihopMembership(
			ctx, connectionEnd, proofHeight, proofInit,
			channel.ConnectionHops, kvGenerator); err != nil {
			return err
		}

	} else {
		counterpartyHops := []string{connectionEnd.GetCounterparty().GetConnectionID()}
		counterparty := types.NewCounterparty(portID, channelID)
		expectedChannel := types.NewChannel(
			types.CLOSED, channel.Ordering, counterparty,
			counterpartyHops, channel.Version,
		)
		if err := k.connectionKeeper.VerifyChannelState(
			ctx, connectionEnd, proofHeight, proofInit,
			channel.Counterparty.PortId, channel.Counterparty.ChannelId,
			expectedChannel,
		); err != nil {
			return err
		}
	}

	k.Logger(ctx).
		Info("channel state updated", "port-id", portID, "channel-id", channelID, "previous-state", channel.State.String(), "new-state", "CLOSED")

	defer func() {
		telemetry.IncrCounter(1, "ibc", "channel", "close-confirm")
	}()

	// Set channel state to CLOSE_CONFIRM_PENDING if the first connection is virtual; otherwise, set it to CLOSED
	isVirtual, _ := k.IsVirtualConnection(ctx, channel.ConnectionHops[0])
	if isVirtual {
		channel.State = types.CLOSE_CONFIRM_PENDING
	} else {
		channel.State = types.CLOSED
	}
	k.SetChannel(ctx, portID, channelID, channel)

	EmitChannelCloseConfirmEvent(ctx, portID, channelID, channel)

	return nil
}

// ChanCloseFrozen is called by the counterparty module to close their end of the
// channel due to a frozen client in the channel path.
func (k Keeper) ChanCloseFrozen(
	ctx sdk.Context,
	portID,
	channelID string,
	chanCap *capabilitytypes.Capability,
	proofFrozen []byte,
	proofHeight exported.Height,
) error {
	if !k.scopedKeeper.AuthenticateCapability(ctx, chanCap, host.ChannelCapabilityPath(portID, channelID)) {
		return sdkerrors.Wrap(
			types.ErrChannelCapabilityNotFound,
			"caller does not own capability for channel, port ID (%s) channel ID (%s)",
		)
	}

	channel, found := k.GetChannel(ctx, portID, channelID)
	if !found {
		return sdkerrors.Wrapf(types.ErrChannelNotFound, "port ID (%s) channel ID (%s)", portID, channelID)
	}

	// ChanCloseFrozen is only required for multi-hop channels
	if len(channel.ConnectionHops) == 1 {
		return sdkerrors.ErrNotSupported
	}

	if channel.State == types.CLOSED {
		return sdkerrors.Wrap(types.ErrInvalidChannelState, "channel is already CLOSED")
	}

	connectionEnd, found := k.connectionKeeper.GetConnection(ctx, channel.ConnectionHops[0])
	if !found {
		return sdkerrors.Wrap(
			connectiontypes.ErrConnectionNotFound,
			channel.ConnectionHops[0],
		)
	}

	if connectionEnd.GetState() != int32(connectiontypes.OPEN) {
		return sdkerrors.Wrapf(
			connectiontypes.ErrInvalidConnectionState,
			"connection state is not OPEN (got %s)", connectiontypes.State(connectionEnd.GetState()).String(),
		)
	}

	var mProof types.MsgMultihopProofs
	if err := k.cdc.Unmarshal(proofFrozen, &mProof); err != nil {
		return fmt.Errorf("cannot unmarshal proof: %v", err)
	}

	kvGenerator := func(mProof *types.MsgMultihopProofs, multihopConnectionEnd *connectiontypes.ConnectionEnd) (string, []byte, error) {
		key := host.FullClientStatePath(multihopConnectionEnd.ClientId)
		value := mProof.KeyProof.Value // client state
		return key, value, nil
	}

	// truncated connectionHops 0, 1, 2, 3 -> 1, 2, 3
	connectionHops := channel.ConnectionHops[:len(mProof.ConnectionProofs)]

	// unmarshal to client state interface
	var exportedClientState exported.ClientState
	if err := k.cdc.UnmarshalInterface(mProof.KeyProof.Value, &exportedClientState); err != nil {
		return err
	}

	// try to cast to tendermint client state
	cs, ok := exportedClientState.(*tmclient.ClientState)
	if !ok {
		return fmt.Errorf("cannot cast exported client state to tendermint client state")
	}

	// check client is frozen
	if cs.FrozenHeight.RevisionHeight != 0 && cs.FrozenHeight.RevisionNumber != 0 {
		return fmt.Errorf("cannot close channel, client is not frozen")
	}

	// prove frozen client
	if err := k.connectionKeeper.VerifyMultihopMembership(
		ctx, connectionEnd, proofHeight, proofFrozen,
		connectionHops, kvGenerator); err != nil {
		return err
	}

	k.Logger(ctx).
		Info("channel state updated", "port-id", portID, "channel-id", channelID, "previous-state", channel.State.String(), "new-state", "CLOSED")

	defer func() {
		telemetry.IncrCounter(1, "ibc", "channel", "close-confirm")
	}()

	channel.State = types.FROZEN
	k.SetChannel(ctx, portID, channelID, channel)

	EmitChannelCloseConfirmEvent(ctx, portID, channelID, channel)

	return nil
}
