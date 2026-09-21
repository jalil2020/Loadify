package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/loadify/backend/internal/domain"
)

// The classification is the contract between Telegram and the worker's rules.
// Every error string here is one Telegram documents or is known to send; a new
// one is a new row.
func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		err  error
		kind domain.TelegramErrorKind
		wait time.Duration
	}{
		{"flood wait", tgerr.New(420, "FLOOD_WAIT_600"), domain.TelegramFloodWait, 10 * time.Minute},
		{"premium flood wait", tgerr.New(420, "FLOOD_PREMIUM_WAIT_30"), domain.TelegramFloodWait, 30 * time.Second},
		{"slow mode", tgerr.New(420, "SLOWMODE_WAIT_120"), domain.TelegramSlowmode, 2 * time.Minute},
		{"spam limit", tgerr.New(400, "PEER_FLOOD"), domain.TelegramPeerFlood, 0},
		{"banned account", tgerr.New(401, "USER_DEACTIVATED_BAN"), domain.TelegramPeerFlood, 0},
		{"write forbidden", tgerr.New(403, "CHAT_WRITE_FORBIDDEN"), domain.TelegramWriteForbidden, 0},
		{"text forbidden", tgerr.New(403, "CHAT_SEND_PLAIN_FORBIDDEN"), domain.TelegramWriteForbidden, 0},
		{"restricted in group", tgerr.New(400, "CHAT_RESTRICTED"), domain.TelegramWriteForbidden, 0},
		// Telegram's wording names a channel, but the ban is on the account:
		// it may not write in any supergroup or channel.
		{"banned from writing in groups", tgerr.New(400, "USER_BANNED_IN_CHANNEL"), domain.TelegramPeerFlood, 0},
		{"left the group", tgerr.New(406, "CHANNEL_PRIVATE"), domain.TelegramGroupGone, 0},
		{"no such channel", tgerr.New(400, "CHANNEL_INVALID"), domain.TelegramGroupGone, 0},
		{"no such peer", tgerr.New(404, "PEER_ID_INVALID"), domain.TelegramGroupGone, 0},
		{"key unregistered", tgerr.New(401, "AUTH_KEY_UNREGISTERED"), domain.TelegramSessionRevoked, 0},
		{"session revoked", tgerr.New(401, "SESSION_REVOKED"), domain.TelegramSessionRevoked, 0},
		{"already sent", tgerr.New(500, "RANDOM_ID_DUPLICATE"), domain.TelegramDuplicate, 0},
		{"too long", tgerr.New(400, "MESSAGE_TOO_LONG"), domain.TelegramMessageTooLong, 0},
		{"server trouble", tgerr.New(500, "INTERNAL_SERVER_ERROR"), domain.TelegramTransient, 0},
		{"network", io.ErrUnexpectedEOF, domain.TelegramTransient, 0},
		{"wrapped rpc error", fmt.Errorf("invoke: %w", tgerr.New(403, "CHAT_WRITE_FORBIDDEN")), domain.TelegramWriteForbidden, 0},
		{"something new", tgerr.New(400, "SOMETHING_NOBODY_EXPECTED"), domain.TelegramUnknown, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.err)
			if got == nil {
				t.Fatal("a non-nil error classified as nil")
			}
			if got.Kind != tc.kind {
				t.Errorf("kind = %q, want %q", got.Kind, tc.kind)
			}
			if got.Wait != tc.wait {
				t.Errorf("wait = %v, want %v", got.Wait, tc.wait)
			}
			if !errors.Is(got, tc.err) && !errors.Is(got.Err, tc.err) {
				t.Error("the original error is not reachable through the classification")
			}
		})
	}
}

// The code a person sees in the history is Telegram's own, argument included,
// because that is what a support conversation will quote.
func TestClassifyKeepsTelegramsCode(t *testing.T) {
	if got := Classify(tgerr.New(420, "FLOOD_WAIT_600")); got.Code != "FLOOD_WAIT_600" {
		t.Errorf("code = %q", got.Code)
	}
}

func TestClassifyNil(t *testing.T) {
	if Classify(nil) != nil {
		t.Error("nil classified as an error")
	}
}

func TestSignInErrorsNameWhatToFix(t *testing.T) {
	cases := map[string]struct {
		err  error
		want error
	}{
		"invalid phone":  {tgerr.New(400, "PHONE_NUMBER_INVALID"), domain.ErrTelegramPhoneInvalid},
		"banned phone":   {tgerr.New(400, "PHONE_NUMBER_BANNED"), domain.ErrTelegramPhoneBanned},
		"no account":     {tgerr.New(400, "PHONE_NUMBER_UNOCCUPIED"), domain.ErrTelegramPhoneUnknown},
		"wrong code":     {tgerr.New(400, "PHONE_CODE_INVALID"), domain.ErrTelegramCodeInvalid},
		"expired code":   {tgerr.New(400, "PHONE_CODE_EXPIRED"), domain.ErrTelegramCodeExpired},
		"wrong password": {auth.ErrPasswordInvalid, domain.ErrTelegramPasswordInvalid},
		"sign-up needed": {&auth.SignUpRequired{}, domain.ErrTelegramPhoneUnknown},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := signInError(tc.err); !errors.Is(got, tc.want) {
				t.Errorf("signInError = %v, want %v", got, tc.want)
			}
		})
	}

	// Anything else still reaches the worker's rules rather than disappearing.
	var classified *domain.TelegramError
	if !errors.As(signInError(tgerr.New(420, "FLOOD_WAIT_60")), &classified) || classified.Kind != domain.TelegramFloodWait {
		t.Error("a flood wait during sign-in lost its meaning")
	}
}

func TestPeerRoundTrip(t *testing.T) {
	hash := int64(987654321)

	channel, err := InputPeer(MarkChannel(1234567890), &hash)
	if err != nil {
		t.Fatalf("InputPeer(channel): %v", err)
	}
	if got, ok := channel.(*tg.InputPeerChannel); !ok || got.ChannelID != 1234567890 || got.AccessHash != hash {
		t.Errorf("channel peer = %#v", channel)
	}
	if MarkChannel(1234567890) != -1001234567890 {
		t.Errorf("marked channel = %d, want the -100 form people see", MarkChannel(1234567890))
	}

	chat, err := InputPeer(MarkChat(4242), nil)
	if err != nil {
		t.Fatalf("InputPeer(chat): %v", err)
	}
	if got, ok := chat.(*tg.InputPeerChat); !ok || got.ChatID != 4242 {
		t.Errorf("chat peer = %#v", chat)
	}
}

func TestPeersTheProductDoesNotPostTo(t *testing.T) {
	if _, err := InputPeer(777000, nil); !errors.Is(err, ErrUnsupportedPeer) {
		t.Errorf("a person's id was accepted as a destination: %v", err)
	}
	if _, err := InputPeer(MarkChannel(1), nil); err == nil {
		t.Error("a channel without its access hash was accepted")
	}
}

// A malformed stored peer is not worth a network round trip.
func TestSendRefusesABadPeerWithoutConnecting(t *testing.T) {
	client := &Client{}
	_, err := client.Send(context.Background(), domain.SendRequest{PeerID: 42})

	var classified *domain.TelegramError
	if !errors.As(err, &classified) || classified.Kind != domain.TelegramGroupGone {
		t.Errorf("err = %v, want the group treated as unreachable", err)
	}
}

func TestSentMessageIDMatchesTheRandomID(t *testing.T) {
	short := &tg.UpdateShortSentMessage{ID: 11}
	if got := sentMessageID(short, 1); got != 11 {
		t.Errorf("short reply id = %d", got)
	}

	full := &tg.Updates{Updates: []tg.UpdateClass{
		&tg.UpdateMessageID{ID: 5, RandomID: 999},
		&tg.UpdateMessageID{ID: 12, RandomID: 424242},
	}}
	if got := sentMessageID(full, 424242); got != 12 {
		t.Errorf("full reply id = %d, want the one carrying our random id", got)
	}
	if got := sentMessageID(full, 1); got != 0 {
		t.Errorf("an unrelated update was taken as ours: %d", got)
	}
}

func elemFor(inputPeer tg.InputPeerClass, chats map[int64]*tg.Chat, channels map[int64]*tg.Channel) dialogs.Elem {
	return dialogs.Elem{Peer: inputPeer, Entities: peer.NewEntities(nil, chats, channels)}
}

func TestDialogsKeepOnlyWritableGroups(t *testing.T) {
	cases := []struct {
		name    string
		elem    dialogs.Elem
		listed  bool
		canSend bool
		kind    string
	}{
		{
			name: "an open supergroup",
			elem: elemFor(&tg.InputPeerChannel{ChannelID: 1, AccessHash: 7}, nil, map[int64]*tg.Channel{
				1: {ID: 1, AccessHash: 7, Title: "Yuk markazi", Megagroup: true, ParticipantsCount: 5400},
			}),
			listed: true, canSend: true, kind: "supergroup",
		},
		{
			name: "a supergroup where members may not write",
			elem: elemFor(&tg.InputPeerChannel{ChannelID: 2, AccessHash: 7}, nil, map[int64]*tg.Channel{
				2: {ID: 2, AccessHash: 7, Megagroup: true, DefaultBannedRights: tg.ChatBannedRights{SendMessages: true}},
			}),
			listed: true, canSend: false, kind: "supergroup",
		},
		{
			name: "the same group, but this account is an admin",
			elem: elemFor(&tg.InputPeerChannel{ChannelID: 3, AccessHash: 7}, nil, map[int64]*tg.Channel{
				3: {ID: 3, AccessHash: 7, Megagroup: true,
					DefaultBannedRights: tg.ChatBannedRights{SendMessages: true},
					AdminRights:         tg.ChatAdminRights{PinMessages: true}},
			}),
			listed: true, canSend: true, kind: "supergroup",
		},
		{
			name: "a channel the account only reads",
			elem: elemFor(&tg.InputPeerChannel{ChannelID: 4, AccessHash: 7}, nil, map[int64]*tg.Channel{
				4: {ID: 4, AccessHash: 7, Broadcast: true},
			}),
			listed: true, canSend: false, kind: "channel",
		},
		{
			name: "a group the account left",
			elem: elemFor(&tg.InputPeerChannel{ChannelID: 5, AccessHash: 7}, nil, map[int64]*tg.Channel{
				5: {ID: 5, AccessHash: 7, Megagroup: true, Left: true},
			}),
			listed: false,
		},
		{
			name: "a basic group that moved to a supergroup",
			elem: elemFor(&tg.InputPeerChat{ChatID: 6}, map[int64]*tg.Chat{
				6: {ID: 6, MigratedTo: &tg.InputChannel{ChannelID: 1, AccessHash: 7}},
			}, nil),
			listed: false,
		},
		{
			name: "an open basic group",
			elem: elemFor(&tg.InputPeerChat{ChatID: 8}, map[int64]*tg.Chat{
				8: {ID: 8, Title: "Andijon yuklari", ParticipantsCount: 120},
			}, nil),
			listed: true, canSend: true, kind: "group",
		},
		{
			name:   "a person",
			elem:   dialogs.Elem{Peer: &tg.InputPeerUser{UserID: 9}},
			listed: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dialog, listed := dialogFrom(tc.elem)
			if listed != tc.listed {
				t.Fatalf("listed = %v, want %v", listed, tc.listed)
			}
			if !listed {
				return
			}
			if dialog.CanSend != tc.canSend {
				t.Errorf("can send = %v, want %v", dialog.CanSend, tc.canSend)
			}
			if dialog.Kind != tc.kind {
				t.Errorf("kind = %q, want %q", dialog.Kind, tc.kind)
			}
		})
	}
}

// A supergroup is stored in the marked form, with its access hash, so a later
// send can address it without asking Telegram again.
func TestADialogCarriesWhatASendNeeds(t *testing.T) {
	dialog, _ := dialogFrom(elemFor(&tg.InputPeerChannel{ChannelID: 1234567890, AccessHash: 55}, nil,
		map[int64]*tg.Channel{1234567890: {ID: 1234567890, AccessHash: 55, Megagroup: true, Username: "yukmarkazi"}}))

	if dialog.PeerID != -1001234567890 {
		t.Errorf("peer id = %d", dialog.PeerID)
	}
	if dialog.AccessHash == nil || *dialog.AccessHash != 55 {
		t.Errorf("access hash = %v", dialog.AccessHash)
	}
	if dialog.Username == nil || *dialog.Username != "yukmarkazi" {
		t.Errorf("username = %v", dialog.Username)
	}
	if _, err := InputPeer(dialog.PeerID, dialog.AccessHash); err != nil {
		t.Errorf("the stored dialog cannot be addressed: %v", err)
	}
}
