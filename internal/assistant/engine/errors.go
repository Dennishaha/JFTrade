package adk

import (
	"errors"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

var (
	errGoogleADKFunctionCallEventMissing = errors.New("no function call event found for function responses ids")
)

// classifiedError preserves the original error text and chain while adding a
// stable sentinel classification. This is needed at GO-ADK boundaries that
// serialize an error into FunctionResponse before JFTrade observes it.
type classifiedError struct {
	cause error
	class error
}

func (e classifiedError) Error() string {
	return e.cause.Error()
}

func (e classifiedError) Unwrap() []error {
	return []error{e.class, e.cause}
}

func withErrorClass(err error, class error) error {
	if err == nil || class == nil || errors.Is(err, class) {
		return err
	}
	return classifiedError{cause: err, class: class}
}

func errorFromSerializedADKValue(value any) error {
	return jfadkmodel.ErrorFromSerializedADKValue(value)
}

func ErrorFromSerializedADKText(text string) error {
	return jfadkmodel.ErrorFromSerializedADKText(text)
}

func classifyGoogleADKRunnerError(err error) error {
	if err == nil || errors.Is(err, errGoogleADKFunctionCallEventMissing) {
		return err
	}
	// GO-ADK v2.0.0 does not expose a sentinel for this replay validation
	// failure. Classify its stable prefix once at the adapter boundary while
	// preserving the upstream error as an unwrap target.
	message := err.Error()
	if strings.HasPrefix(message, errGoogleADKFunctionCallEventMissing.Error()) {
		return withErrorClass(err, errGoogleADKFunctionCallEventMissing)
	}
	return err
}
