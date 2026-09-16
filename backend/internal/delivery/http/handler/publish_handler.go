package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/usecase"
	"github.com/loadify/backend/pkg/pagination"
	"github.com/loadify/backend/pkg/response"
	"github.com/loadify/backend/pkg/validator"
)

// idempotencyHeader is what makes a retried publish safe. A network timeout on
// the way back is indistinguishable from a failure, and the client's only
// sensible move is to try again.
const idempotencyHeader = "Idempotency-Key"

// PublishHandler exposes distribution: what would be sent, what was sent, and
// stopping what has not been sent yet.
type PublishHandler struct {
	publish   *usecase.PublishUsecase
	validator validator.Validator
}

// NewPublishHandler builds the publish handler.
func NewPublishHandler(publish *usecase.PublishUsecase, v validator.Validator) *PublishHandler {
	return &PublishHandler{publish: publish, validator: v}
}

// Preview works out where an advert would go and why each group was or was not
// chosen. It sends nothing and consumes no allowance.
// POST /api/v1/load-ads/:id/publish/preview
func (h *PublishHandler) Preview(c *fiber.Ctx) error {
	var q dto.PublishPreviewQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	adID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}

	res, err := h.publish.Preview(c.UserContext(), userID, companyID, adID, q.AccountID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Publish queues the advert for the groups the caller just reviewed.
// POST /api/v1/load-ads/:id/publish
func (h *PublishHandler) Publish(c *fiber.Ctx) error {
	var req dto.PublishRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	adID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}

	res, err := h.publish.Publish(c.UserContext(), userID, companyID, adID, c.Get(idempotencyHeader), req)
	if err != nil {
		return err
	}
	// Accepted rather than created: the jobs exist, the messages have not been
	// sent yet, and pretending otherwise would misdescribe what happened.
	return c.Status(fiber.StatusAccepted).JSON(response.Envelope{Success: true, Data: res})
}

// Jobs is the per-advert view: one line per group, with what happened.
// GET /api/v1/load-ads/:id/posts
func (h *PublishHandler) Jobs(c *fiber.Ctx) error {
	_, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	adID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}

	res, err := h.publish.JobsForAd(c.UserContext(), companyID, adID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// History lists what went where, newest first.
// GET /api/v1/posting-history
func (h *PublishHandler) History(c *fiber.Ctx) error {
	var q dto.PostingHistoryQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	page := pagination.Parse(c)

	items, total, err := h.publish.History(c.UserContext(), userID, companyID, q, page.Offset(), page.Limit())
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// Cancel stops a post that has not gone out yet.
// POST /api/v1/posting-jobs/:id/cancel
func (h *PublishHandler) Cancel(c *fiber.Ctx) error {
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	jobID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}

	if err := h.publish.CancelJob(c.UserContext(), userID, companyID, jobID); err != nil {
		return err
	}
	return response.NoContent(c)
}
