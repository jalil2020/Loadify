package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/validator"
)

// bind parses the JSON body into dst and validates it. Every handler starts here,
// so malformed input never reaches a usecase.
func bind(c *fiber.Ctx, dst any, v validator.Validator) error {
	if err := c.BodyParser(dst); err != nil {
		return apperror.ErrBadRequest.
			WithMessage("Request body is not valid JSON").Wrap(err)
	}
	return v.Struct(dst)
}

// bindQuery parses query parameters into dst and validates them.
func bindQuery(c *fiber.Ctx, dst any, v validator.Validator) error {
	if err := c.QueryParser(dst); err != nil {
		return apperror.ErrBadRequest.
			WithMessage("Invalid query parameters").Wrap(err)
	}
	return v.Struct(dst)
}

// paramUUID reads a UUID path parameter.
func paramUUID(c *fiber.Ctx, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params(name))
	if err != nil {
		return uuid.Nil, apperror.ErrBadRequest.
			WithMessage("Invalid " + name).Wrap(err)
	}
	return id, nil
}
