package validator

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

type Validator struct {
	validate *validator.Validate
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type Errors struct {
	Fields []FieldError `json:"fields"`
}

func (e Errors) Error() string {
	if len(e.Fields) == 0 {
		return "validation failed"
	}

	messages := make([]string, 0, len(e.Fields))

	for _, field := range e.Fields {
		messages = append(
			messages,
			fmt.Sprintf("%s: %s", field.Field, field.Message),
		)
	}

	return strings.Join(messages, "; ")
}

func New() *Validator {
	v := validator.New()

	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.Split(field.Tag.Get("json"), ",")[0]

		if name == "-" {
			return ""
		}

		return name
	})

	return &Validator{
		validate: v,
	}
}

func (v *Validator) Validate(value any) error {
	if err := v.validate.Struct(value); err != nil {
		var validationErrors validator.ValidationErrors

		if !errors.As(err, &validationErrors) {
			return fmt.Errorf("validate struct: %w", err)
		}

		return Errors{
			Fields: formatErrors(validationErrors),
		}
	}

	return nil
}

func formatErrors(errs validator.ValidationErrors) []FieldError {
	result := make([]FieldError, 0, len(errs))

	for _, err := range errs {
		result = append(result, FieldError{
			Field:   err.Field(),
			Message: message(err),
		})
	}

	return result
}

func message(err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return "is required"

	case "email":
		return "must be a valid email address"

	case "min":
		return fmt.Sprintf("must be at least %s characters", err.Param())

	case "max":
		return fmt.Sprintf("must be at most %s characters", err.Param())

	case "oneof":
		return fmt.Sprintf("must be one of: %s", err.Param())

	case "uuid":
		return "must be a valid UUID"

	default:
		return "is invalid"
	}
}
