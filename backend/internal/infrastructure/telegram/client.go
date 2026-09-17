package telegram

import (
	"context"
	"errors"
	"fmt"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
	"github.com/loadify/backend/pkg/secret"
)

// Client implements domain.TelegramSender and domain.TelegramAuthenticator
// over MTProto.
//
// Each call opens a connection from the stored session, does its one thing and
// closes it. That is slower than holding a connection per account, and it is
// the right first shape: a long-lived connection is a second place where an
// account's state lives, and every question this package answers is easier
// when the session in the database is the only one.
//
// No flood-wait middleware is installed on purpose. When Telegram asks for a
// wait, the right response is to pause the account and reschedule its work —
// a decision for the worker, not a sleep inside a request.
type Client struct {
	appID   int
	appHash secret.Value
	device  telegram.DeviceConfig
	log     logger.Logger
}

// New builds the client from configuration.
func New(cfg config.TelegramConfig, log logger.Logger) *Client {
	return &Client{
		appID:   cfg.APIID,
		appHash: secret.Value(cfg.APIHash),
		device: telegram.DeviceConfig{
			// What the person sees under Settings → Devices. It should say
			// plainly which product holds the session, so revoking it is easy.
			DeviceModel:    cfg.DeviceModel,
			SystemVersion:  "server",
			AppVersion:     cfg.AppVersion,
			SystemLangCode: "ru",
			LangCode:       "ru",
		},
		log: log,
	}
}

// run opens a connection from a session, runs fn, and returns the session as
// Telegram left it — which differs from the one passed in after a data-centre
// migration or on the first authorisation.
func (c *Client) run(
	ctx context.Context,
	stored []byte,
	fn func(ctx context.Context, client *telegram.Client) error,
) ([]byte, error) {
	storage := &session.StorageMemory{}
	if len(stored) > 0 {
		// A copy, so the caller wiping its buffer does not pull the session out
		// from under a connection that is still using it.
		if err := storage.StoreSession(ctx, append([]byte(nil), stored...)); err != nil {
			return nil, fmt.Errorf("telegram: load session: %w", err)
		}
	}

	client := telegram.NewClient(c.appID, c.appHash.Reveal(), telegram.Options{
		SessionStorage: storage,
		NoUpdates:      true,
		Device:         c.device,
	})
	if err := client.Run(ctx, func(ctx context.Context) error { return fn(ctx, client) }); err != nil {
		return nil, err
	}

	updated, err := storage.LoadSession(ctx)
	if errors.Is(err, session.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("telegram: read session: %w", err)
	}
	return updated, nil
}

// Send posts one message into one group.
func (c *Client) Send(ctx context.Context, req domain.SendRequest) (domain.SendResult, error) {
	peer, err := InputPeer(req.PeerID, req.AccessHash)
	if err != nil {
		// The stored address is unusable, which for the worker means the same
		// as a group the account can no longer reach.
		return domain.SendResult{}, &domain.TelegramError{Kind: domain.TelegramGroupGone, Code: "invalid_peer", Err: err}
	}

	var result domain.SendResult
	_, err = c.run(ctx, req.Session, func(ctx context.Context, client *telegram.Client) error {
		updates, err := client.API().MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer:     peer,
			Message:  req.Text,
			RandomID: req.RandomID,
			// A link preview would turn the advert into a card for whatever
			// URL someone put in the notes. The message is the advert.
			NoWebpage: true,
			// AllowPaidFloodskip is deliberately never set: paying to exceed
			// Telegram's limits is the one thing this product must not do.
		})
		if err != nil {
			return err
		}
		result.MessageID = sentMessageID(updates, req.RandomID)
		return nil
	})
	if err != nil {
		return domain.SendResult{}, Classify(err)
	}
	return result, nil
}

// sentMessageID finds the id Telegram gave the message, matched by random_id so
// an unrelated update in the same reply is never mistaken for it.
func sentMessageID(updates tg.UpdatesClass, randomID int64) int64 {
	switch u := updates.(type) {
	case *tg.UpdateShortSentMessage:
		return int64(u.ID)
	case *tg.Updates:
		for _, update := range u.Updates {
			if sent, ok := update.(*tg.UpdateMessageID); ok && sent.RandomID == randomID {
				return int64(sent.ID)
			}
		}
	}
	return 0
}

// SendCode asks Telegram to send a login code to the person's own app.
func (c *Client) SendCode(ctx context.Context, phone string) (domain.TelegramLogin, error) {
	login := domain.TelegramLogin{Phone: phone}

	stored, err := c.run(ctx, nil, func(ctx context.Context, client *telegram.Client) error {
		sent, err := client.Auth().SendCode(ctx, phone, auth.SendCodeOptions{})
		if err != nil {
			return err
		}
		code, ok := sent.(*tg.AuthSentCode)
		if !ok {
			return fmt.Errorf("telegram: unexpected answer to a code request: %T", sent)
		}
		login.CodeHash = code.PhoneCodeHash
		return nil
	})
	if err != nil {
		return login, signInError(err)
	}
	login.Session = stored
	return login, nil
}

// SignIn submits the code the person received.
func (c *Client) SignIn(ctx context.Context, login domain.TelegramLogin, code string) (domain.TelegramLoginResult, error) {
	var result domain.TelegramLoginResult

	stored, err := c.run(ctx, login.Session, func(ctx context.Context, client *telegram.Client) error {
		authorization, err := client.Auth().SignIn(ctx, login.Phone, code, login.CodeHash)
		if errors.Is(err, auth.ErrPasswordAuthNeeded) {
			result.PasswordRequired = true
			return nil
		}
		if err != nil {
			return err
		}
		result.TgUserID = userID(authorization)
		return nil
	})
	if err != nil {
		return result, signInError(err)
	}
	result.Session = stored
	return result, nil
}

// CheckPassword completes a sign-in on an account with two-step verification.
//
// The password is used for the SRP exchange and dropped with this call. It is
// never stored and never logged.
func (c *Client) CheckPassword(ctx context.Context, login domain.TelegramLogin, password string) (domain.TelegramLoginResult, error) {
	var result domain.TelegramLoginResult

	stored, err := c.run(ctx, login.Session, func(ctx context.Context, client *telegram.Client) error {
		authorization, err := client.Auth().Password(ctx, password)
		if err != nil {
			return err
		}
		result.TgUserID = userID(authorization)
		return nil
	})
	if err != nil {
		return result, signInError(err)
	}
	result.Session = stored
	return result, nil
}

func userID(authorization *tg.AuthAuthorization) int64 {
	if authorization == nil {
		return 0
	}
	if user, ok := authorization.User.(*tg.User); ok {
		return user.ID
	}
	return 0
}

// signInError names the failures a person can do something about.
func signInError(err error) error {
	switch {
	case errors.Is(err, auth.ErrPasswordInvalid):
		return domain.ErrTelegramPasswordInvalid
	case tgerr.Is(err, "PHONE_NUMBER_INVALID"):
		return domain.ErrTelegramPhoneInvalid
	case tgerr.Is(err, "PHONE_NUMBER_BANNED"):
		return domain.ErrTelegramPhoneBanned
	case tgerr.Is(err, "PHONE_NUMBER_UNOCCUPIED"):
		return domain.ErrTelegramPhoneUnknown
	case tgerr.Is(err, "PHONE_CODE_INVALID", "PHONE_CODE_EMPTY"):
		return domain.ErrTelegramCodeInvalid
	case tgerr.Is(err, "PHONE_CODE_EXPIRED"):
		return domain.ErrTelegramCodeExpired
	case tgerr.Is(err, "PASSWORD_HASH_INVALID"):
		return domain.ErrTelegramPasswordInvalid
	}
	var signUp *auth.SignUpRequired
	if errors.As(err, &signUp) {
		// Creating a Telegram account for someone is not something this
		// product does.
		return domain.ErrTelegramPhoneUnknown
	}
	return Classify(err)
}

// Dialogs lists the groups the account is a member of.
func (c *Client) Dialogs(ctx context.Context, stored []byte) ([]domain.TelegramDialog, error) {
	var found []domain.TelegramDialog

	_, err := c.run(ctx, stored, func(ctx context.Context, client *telegram.Client) error {
		iter := dialogs.NewQueryBuilder(client.API()).GetDialogs().BatchSize(100).Iter()
		for iter.Next(ctx) {
			if dialog, ok := dialogFrom(iter.Value()); ok {
				found = append(found, dialog)
			}
		}
		return iter.Err()
	})
	if err != nil {
		return nil, Classify(err)
	}
	return found, nil
}

// dialogFrom keeps groups and channels and says whether the account may write.
func dialogFrom(elem dialogs.Elem) (domain.TelegramDialog, bool) {
	switch peer := elem.Peer.(type) {
	case *tg.InputPeerChat:
		chat, ok := elem.Entities.Chat(peer.ChatID)
		// A migrated basic group lives on as a supergroup, which appears in
		// the list on its own; the old one can no longer be written to.
		if !ok || chat.Left || chat.Deactivated || chat.MigratedTo != nil {
			return domain.TelegramDialog{}, false
		}
		banned := chat.DefaultBannedRights.SendMessages || chat.DefaultBannedRights.SendPlain
		return domain.TelegramDialog{
			PeerID:      MarkChat(chat.ID),
			Title:       chat.Title,
			Kind:        "group",
			MemberCount: optionalCount(chat.ParticipantsCount),
			CanSend:     chat.Creator || hasAdminRights(chat.AdminRights) || !banned,
		}, true

	case *tg.InputPeerChannel:
		channel, ok := elem.Entities.Channel(peer.ChannelID)
		if !ok || channel.Left {
			return domain.TelegramDialog{}, false
		}
		dialog := domain.TelegramDialog{
			PeerID:          MarkChannel(channel.ID),
			AccessHash:      &channel.AccessHash,
			Title:           channel.Title,
			Kind:            "supergroup",
			MemberCount:     optionalCount(channel.ParticipantsCount),
			SlowmodeEnabled: channel.SlowmodeEnabled,
		}
		if channel.Username != "" {
			username := channel.Username
			dialog.Username = &username
		}

		admin := channel.Creator || hasAdminRights(channel.AdminRights)
		if channel.Broadcast {
			// A channel is a broadcast: only someone with the right to post
			// may write there, and everyone else is a reader.
			dialog.Kind = "channel"
			dialog.CanSend = channel.Creator || channel.AdminRights.PostMessages
			return dialog, true
		}
		banned := channel.BannedRights.SendMessages || channel.BannedRights.SendPlain ||
			channel.DefaultBannedRights.SendMessages || channel.DefaultBannedRights.SendPlain
		dialog.CanSend = admin || !banned
		return dialog, true

	default:
		// People and bots are not destinations for adverts.
		return domain.TelegramDialog{}, false
	}
}

func hasAdminRights(rights tg.ChatAdminRights) bool {
	return rights.PostMessages || rights.BanUsers || rights.DeleteMessages ||
		rights.ChangeInfo || rights.PinMessages || rights.InviteUsers || rights.Other
}

func optionalCount(n int) *int {
	if n <= 0 {
		return nil
	}
	return &n
}

// SlowmodeSeconds reads a supergroup's slow mode, which the dialog list only
// reports as on or off. The seconds are what the cooldown has to respect.
func (c *Client) SlowmodeSeconds(ctx context.Context, stored []byte, peerID int64, accessHash *int64) (int, error) {
	channel, ok := inputChannel(peerID, accessHash)
	if !ok {
		// Basic groups have no slow mode.
		return 0, nil
	}

	var seconds int
	_, err := c.run(ctx, stored, func(ctx context.Context, client *telegram.Client) error {
		full, err := client.API().ChannelsGetFullChannel(ctx, channel)
		if err != nil {
			return err
		}
		if details, ok := full.FullChat.(*tg.ChannelFull); ok {
			seconds = details.SlowmodeSeconds
		}
		return nil
	})
	if err != nil {
		return 0, Classify(err)
	}
	return seconds, nil
}

// LogOut ends the session on Telegram's side.
func (c *Client) LogOut(ctx context.Context, stored []byte) error {
	_, err := c.run(ctx, stored, func(ctx context.Context, client *telegram.Client) error {
		_, err := client.API().AuthLogOut(ctx)
		return err
	})
	if err != nil {
		classified := Classify(err)
		// A session Telegram already ended is the outcome we wanted.
		if classified.Kind == domain.TelegramSessionRevoked {
			return nil
		}
		return classified
	}
	return nil
}

// The client is both halves of the account's life on the platform: connecting
// it and posting with it.
var (
	_ domain.TelegramSender        = (*Client)(nil)
	_ domain.TelegramAuthenticator = (*Client)(nil)
)
