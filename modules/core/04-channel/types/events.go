package types

import (
	"fmt"

	ibcexported "github.com/cosmos/ibc-go/v7/modules/core/exported"
)

// IBC channel events
const (
	AttributeKeyConnectionID       = "connection_id"
	AttributeKeyPortID             = "port_id"
	AttributeKeyChannelID          = "channel_id"
	AttributeVersion               = "version"
	AttributeCounterpartyPortID    = "counterparty_port_id"
	AttributeCounterpartyChannelID = "counterparty_channel_id"

	EventTypeSendPacket           = "send_packet"
	EventTypeRecvPacket           = "recv_packet"
	EventTypeWriteAck             = "write_acknowledgement"
	EventTypeAcknowledgePacket    = "acknowledge_packet"
	EventTypeTimeoutPacket        = "timeout_packet"
	EventTypeTimeoutPacketOnClose = "timeout_on_close_packet"

	// Deprecated: in favor of AttributeKeyDataHex
	AttributeKeyData = "packet_data"
	// Deprecated: in favor of AttributeKeyAckHex
	AttributeKeyAck = "packet_ack"

	AttributeKeyDataHex          = "packet_data_hex"
	AttributeKeyAckHex           = "packet_ack_hex"
	AttributeKeyTimeoutHeight    = "packet_timeout_height"
	AttributeKeyTimeoutTimestamp = "packet_timeout_timestamp"
	AttributeKeySequence         = "packet_sequence"
	AttributeKeySrcPort          = "packet_src_port"
	AttributeKeySrcChannel       = "packet_src_channel"
	AttributeKeyDstPort          = "packet_dst_port"
	AttributeKeyDstChannel       = "packet_dst_channel"
	AttributeKeyChannelOrdering  = "packet_channel_ordering"
	AttributeKeyConnection       = "packet_connection"
)

// IBC channel events vars
var (
	EventTypeChannelOpenInit            = "channel_open_init"
	EventTypeChannelOpenTryPending      = "channel_open_try_pending"
	EventTypeChannelOpenTry             = "channel_open_try"
	EventTypeChannelOpenAck             = "channel_open_ack"
	EventTypeChannelOpenAckPending      = "channel_open_ack_pending"
	EventTypeChannelOpenConfirm         = "channel_open_confirm"
	EventTypeChannelOpenConfirmPending  = "channel_open_confirm_pending"
	EventTypeChannelCloseInit           = "channel_close_init"
	EventTypeChannelCloseConfirm        = "channel_close_confirm"
	EventTypeChannelClosed              = "channel_close"
	EventTypeChannelCloseConfirmPending = "channel_close_confirm_pending"

	AttributeValueCategory = fmt.Sprintf("%s_%s", ibcexported.ModuleName, SubModuleName)
)
