// Package validator wraps go-playground/validator and turns its errors into
// client-friendly, JSON-serialisable field errors.
package validator

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
	"github.com/go-playground/validator/v10/non-standard/validators"
)

type Validator struct {
	validate *validator.Validate
}

// FieldError describes one invalid field, using the JSON/query name.
type FieldError struct {
	Field   string `json:"field" example:"title"`
	Message string `json:"message" example:"must be at least 3 characters"`
}

// Errors is returned when validation fails.
type Errors struct {
	Fields []FieldError `json:"fields"`
}

func (e Errors) Error() string {
	if len(e.Fields) == 0 {
		return "validation failed"
	}
	msgs := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		msgs = append(msgs, f.Field+": "+f.Message)
	}
	return strings.Join(msgs, "; ")
}

func New() *Validator {
	v := validator.New(validator.WithRequiredStructEnabled())

	// Report fields by the name clients send (json or query tag).
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		for _, tag := range []string{"json", "query"} {
			name, _, _ := strings.Cut(f.Tag.Get(tag), ",")
			if name == "-" {
				return ""
			}
			if name != "" {
				return name
			}
		}
		return f.Name
	})

	// "notblank" rejects whitespace-only strings such as "   ".
	_ = v.RegisterValidation("notblank", validators.NotBlank)

	return &Validator{validate: v}
}

// Validate validates a struct (or pointer to struct).
func (v *Validator) Validate(value any) error {
	err := v.validate.Struct(value)
	if err == nil {
		return nil
	}

	verrs, ok := errors.AsType[validator.ValidationErrors](err)
	if !ok {
		return fmt.Errorf("validate: %w", err)
	}

	fields := make([]FieldError, 0, len(verrs))
	for _, fe := range verrs {
		fields = append(fields, FieldError{Field: fe.Field(), Message: message(fe)})
	}
	return Errors{Fields: fields}
}

// IsUUID reports whether s is a valid UUID.
func (v *Validator) IsUUID(s string) bool {
	return v.validate.Var(s, "required,uuid") == nil
}

func message(fe validator.FieldError) string {
	isString := fe.Kind() == reflect.String

	switch fe.Tag() {
	case "required":
		return "is required"
	case "required_without":
		return "is required when " + snake(fe.Param()) + " is not set"
	case "excluded_with":
		return "cannot be combined with " + snake(fe.Param())
	case "email":
		return "must be a valid email address"
	case "uuid":
		return "must be a valid UUID"
	case "oneof":
		return "must be one of: " + strings.ReplaceAll(fe.Param(), " ", ", ")
	case "min":
		if isString {
			return "must be at least " + fe.Param() + " characters"
		}
		return "must be at least " + fe.Param()
	case "max":
		if isString {
			return "must be at most " + fe.Param() + " characters"
		}
		return "must be at most " + fe.Param()
	case "gte":
		return "must be greater than or equal to " + fe.Param()
	case "lte":
		return "must be less than or equal to " + fe.Param()
	case "nefield":
		return "must differ from " + snake(fe.Param())
	case "notblank":
		return "must not be blank"
	default:
		return "is invalid"
	}
}

// snake converts a Go field name (UserID) to its JSON form (user_id).
func snake(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		upper := unicode.IsUpper(r)
		if upper && i > 0 && (unicode.IsLower(runes[i-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
