// Package response builds the single JSON envelope every endpoint returns.
package response

import (
	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/pkg/apperror"
)

// Meta carries pagination information for list endpoints.
type Meta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// Envelope is the success shape.
type Envelope struct {
	Success bool  `json:"success"`
	Data    any   `json:"data,omitempty"`
	Meta    *Meta `json:"meta,omitempty"`
}

// ErrorEnvelope is the failure shape.
type ErrorEnvelope struct {
	Success bool      `json:"success"`
	Error   ErrorBody `json:"error"`
	TraceID string    `json:"trace_id,omitempty"`
}

// ErrorBody is the machine-readable error payload.
type ErrorBody struct {
	Code    string                `json:"code"`
	Message string                `json:"message"`
	Details []apperror.FieldError `json:"details,omitempty"`
}

// OK writes 200 with a data payload.
func OK(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusOK).JSON(Envelope{Success: true, Data: data})
}

// Created writes 201 with a data payload.
func Created(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusCreated).JSON(Envelope{Success: true, Data: data})
}

// NoContent writes 204.
func NoContent(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

// Paginated writes 200 with data and pagination meta.
func Paginated(c *fiber.Ctx, data any, meta Meta) error {
	return c.Status(fiber.StatusOK).JSON(Envelope{Success: true, Data: data, Meta: &meta})
}

// Fail writes the error envelope. Used by the central error handler.
func Fail(c *fiber.Ctx, err *apperror.Error, traceID string) error {
	return c.Status(err.HTTPStatus).JSON(ErrorEnvelope{
		Success: false,
		Error: ErrorBody{
			Code:    err.Code,
			Message: err.Message,
			Details: err.Details,
		},
		TraceID: traceID,
	})
}
