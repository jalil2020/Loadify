package logger_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/loadify/backend/pkg/logger"
	"github.com/loadify/backend/pkg/secret"
)

const session = "1AgAOMTQ5LjE1NC4xNjcuNTEBu" // shaped like an MTProto session string

// The account struct is logged the way a handler would log it: whole, by
// someone who is not thinking about which field is sensitive.
type account struct {
	ID      string
	Phone   string
	Session secret.Value
	Code    secret.Value
}

func TestSecretsNeverReachTheLog(t *testing.T) {
	for _, env := range []string{"development", "production"} {
		t.Run(env, func(t *testing.T) {
			var out bytes.Buffer
			log := logger.NewWithWriter(env, "debug", &out)

			acc := account{
				ID:      "9f1c",
				Phone:   secret.MaskPhone("+998901234567"),
				Session: secret.Value(session),
				Code:    secret.Value("51724"),
			}
			log.Info("telegram account connected", logger.Any("account", acc))
			log.Error("login failed",
				logger.Err(errors.New("auth: bad code")),
				logger.String("code", acc.Code.String()))

			written := out.String()
			for _, leaked := range []string{session, "51724", "+998901234567"} {
				if strings.Contains(written, leaked) {
					t.Errorf("log line contains %q:\n%s", leaked, written)
				}
			}
			if !strings.Contains(written, "+99890 *** ** 67") {
				t.Errorf("the masked phone is missing, so the line is unusable for support:\n%s", written)
			}
		})
	}
}
