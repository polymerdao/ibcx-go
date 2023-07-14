package host

import "fmt"

const (
	KeyNextSeqSendProcessedPrefix = "nextSeqSendProcessed"
)

// NextSequenceSendProcessedPath returns the path under which the packet send sequence processed boolean is stored.
// This is used to prevent virtual packet replay attacks.
func NextSequenceSendProcessedPath(portID, channelID string, sequence uint64) string {
	return fmt.Sprintf("%s/%s/%s/%d", KeyNextSeqSendProcessedPrefix, portID, channelID, sequence)
}

// NextSequenceSendProcessedKey returns the key under which the packet send sequence processed boolean is stored.
// This is used to prevent virtual packet replay attacks.
func NextSequenceSendProcessedKey(portID, channelID string, sequence uint64) []byte {
	return []byte(NextSequenceSendProcessedPath(portID, channelID, sequence))
}
