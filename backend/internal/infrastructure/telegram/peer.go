package telegram

import (
	"errors"

	"github.com/gotd/td/tg"
)

// Peers are stored in the marked form the Bot API and most Telegram tools show:
// -100 followed by the id for supergroups and channels, the negated id for
// basic groups. It is the form a person can recognise when they compare the
// platform with their own client, and it keeps the two kinds from colliding in
// one column.
const channelMarker = 1_000_000_000_000

// ErrUnsupportedPeer means the stored id is not a group the platform posts to.
var ErrUnsupportedPeer = errors.New("telegram: only groups and channels can be posted to")

// MarkChannel returns the stored form of a supergroup or channel id.
func MarkChannel(id int64) int64 { return -(channelMarker + id) }

// MarkChat returns the stored form of a basic group id.
func MarkChat(id int64) int64 { return -id }

// InputPeer turns a stored id into what the API expects.
func InputPeer(peerID int64, accessHash *int64) (tg.InputPeerClass, error) {
	switch {
	case peerID <= -channelMarker:
		if accessHash == nil {
			// A channel cannot be addressed without the hash the account was
			// given for it; guessing one would only be refused.
			return nil, errors.New("telegram: channel access hash is missing")
		}
		return &tg.InputPeerChannel{ChannelID: -peerID - channelMarker, AccessHash: *accessHash}, nil
	case peerID < 0:
		return &tg.InputPeerChat{ChatID: -peerID}, nil
	default:
		return nil, ErrUnsupportedPeer
	}
}

// inputChannel is the same address for the calls that take a channel rather
// than a peer.
func inputChannel(peerID int64, accessHash *int64) (*tg.InputChannel, bool) {
	if peerID > -channelMarker || accessHash == nil {
		return nil, false
	}
	return &tg.InputChannel{ChannelID: -peerID - channelMarker, AccessHash: *accessHash}, true
}
