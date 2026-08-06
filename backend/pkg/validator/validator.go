// Package validator wraps go-playground/validator and turns its output into
// apperror field details, so handlers never format validation messages.
package validator

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	govalidator "github.com/go-playground/validator/v10"

	"github.com/loadify/backend/pkg/apperror"
)

// Validator validates request structs.
type Validator interface {
	Struct(s any) error
}

type validatorImpl struct{ v *govalidator.Validate }

var (
	// Uzbek mobile numbers: +998 followed by 9 digits.
	uzPhoneRe = regexp.MustCompile(`^\+998[0-9]{9}$`)
	// Any E.164 number, for the international phase.
	e164Re = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
	// Uzbek vehicle plates, e.g. 01A123BC or 01123ABC.
	plateRe = regexp.MustCompile(`^[0-9]{2}[A-Z0-9]{3,8}$`)
)

// New builds the validator with the platform's custom rules registered.
func New() Validator {
	v := govalidator.New(govalidator.WithRequiredStructEnabled())

	// Report the JSON name so clients can map errors back to their fields.
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name, _, _ := strings.Cut(fld.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			return fld.Name
		}
		return name
	})

	_ = v.RegisterValidation("uzphone", func(fl govalidator.FieldLevel) bool {
		return uzPhoneRe.MatchString(fl.Field().String())
	})
	_ = v.RegisterValidation("phone", func(fl govalidator.FieldLevel) bool {
		return e164Re.MatchString(fl.Field().String())
	})
	_ = v.RegisterValidation("plate", func(fl govalidator.FieldLevel) bool {
		return plateRe.MatchString(strings.ToUpper(fl.Field().String()))
	})
	_ = v.RegisterValidation("password", func(fl govalidator.FieldLevel) bool {
		s := fl.Field().String()
		if len(s) < 8 || len(s) > 72 {
			return false
		}
		var hasLetter, hasDigit bool
		for _, r := range s {
			switch {
			case r >= '0' && r <= '9':
				hasDigit = true
			case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
				hasLetter = true
			}
		}
		return hasLetter && hasDigit
	})
	_ = v.RegisterValidation("locale", func(fl govalidator.FieldLevel) bool {
		switch fl.Field().String() {
		case "uz", "ru", "en":
			return true
		}
		return false
	})

	return &validatorImpl{v: v}
}

func (val *validatorImpl) Struct(s any) error {
	err := val.v.Struct(s)
	if err == nil {
		return nil
	}

	var invalid *govalidator.InvalidValidationError
	if ok := asInvalid(err, &invalid); ok {
		return apperror.ErrInternal.Wrap(err)
	}

	var verrs govalidator.ValidationErrors
	if ok := asValidationErrors(err, &verrs); !ok {
		return apperror.ErrValidation.Wrap(err)
	}

	details := make([]apperror.FieldError, 0, len(verrs))
	for _, fe := range verrs {
		details = append(details, apperror.FieldError{
			Field:   fieldPath(fe.Namespace()),
			Message: message(fe),
		})
	}
	return apperror.ErrValidation.WithDetails(details...)
}

// fieldPath strips the root struct name from validator's namespace, so
// "CreateLoadRequest.pickup.lat" becomes "pickup.lat".
func fieldPath(ns string) string {
	if i := strings.Index(ns, "."); i >= 0 && i+1 < len(ns) {
		return ns[i+1:]
	}
	return ns
}

func message(fe govalidator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "This field is required"
	case "email":
		return "Must be a valid email address"
	case "uzphone":
		return "Must be an Uzbek phone number in the format +998XXXXXXXXX"
	case "phone":
		return "Must be a valid international phone number"
	case "plate":
		return "Must be a valid vehicle plate number"
	case "password":
		return "Must be 8-72 characters and contain both letters and digits"
	case "locale":
		return "Must be one of: uz, ru, en"
	case "min":
		return fmt.Sprintf("Must be at least %s", fe.Param())
	case "max":
		return fmt.Sprintf("Must be at most %s", fe.Param())
	case "gt":
		return fmt.Sprintf("Must be greater than %s", fe.Param())
	case "gte":
		return fmt.Sprintf("Must be greater than or equal to %s", fe.Param())
	case "lt":
		return fmt.Sprintf("Must be less than %s", fe.Param())
	case "lte":
		return fmt.Sprintf("Must be less than or equal to %s", fe.Param())
	case "len":
		return fmt.Sprintf("Must be exactly %s characters", fe.Param())
	case "oneof":
		return fmt.Sprintf("Must be one of: %s", strings.ReplaceAll(fe.Param(), " ", ", "))
	case "uuid", "uuid4":
		return "Must be a valid identifier"
	case "url":
		return "Must be a valid URL"
	case "eqfield":
		return fmt.Sprintf("Must match %s", fe.Param())
	case "nefield":
		return fmt.Sprintf("Must differ from %s", fe.Param())
	case "latitude":
		return "Must be a valid latitude between -90 and 90"
	case "longitude":
		return "Must be a valid longitude between -180 and 180"
	case "gtfield":
		return fmt.Sprintf("Must be later than %s", fe.Param())
	default:
		return fmt.Sprintf("Failed the '%s' rule", fe.Tag())
	}
}

func asInvalid(err error, target **govalidator.InvalidValidationError) bool {
	v, ok := err.(*govalidator.InvalidValidationError)
	if ok {
		*target = v
	}
	return ok
}

func asValidationErrors(err error, target *govalidator.ValidationErrors) bool {
	v, ok := err.(govalidator.ValidationErrors)
	if ok {
		*target = v
	}
	return ok
}
