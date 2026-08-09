package model

import (
	"errors"
	"fmt"
	"strings"

	adktool "google.golang.org/adk/v2/tool"
	adkworkflow "google.golang.org/adk/v2/workflow"
)

// ErrUserGoalPauseRequested is the sentinel returned when a goal workflow turn
// must stop before another model call because the user asked to pause.
var ErrUserGoalPauseRequested = errors.New("user goal pause requested")

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

func classifySerializedADKError(err error) error {
	if err == nil {
		return nil
	}
	// GO-ADK FunctionResponse and persisted ToolCall records carry only the
	// rendered error. Rehydrate only the sentinels whose text is owned by GO-ADK
	// or this package, so all business call sites can use errors.Is.
	for _, class := range []error{
		adktool.ErrConfirmationRequired,
		adktool.ErrConfirmationRejected,
		adkworkflow.ErrNodeInterrupted,
		ErrUserGoalPauseRequested,
	} {
		if errors.Is(err, class) {
			return err
		}
		if strings.Contains(err.Error(), class.Error()) {
			return withErrorClass(err, class)
		}
	}
	return err
}

// ErrorFromSerializedADKText rehydrates a serialized GO-ADK error into the
// canonical sentinel chain when the rendered text belongs to a known class.
func ErrorFromSerializedADKText(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return classifySerializedADKError(errors.New(text))
}

// ErrorFromSerializedADKValue rehydrates a serialized GO-ADK error value,
// preserving real error instances and classifying rendered text otherwise.
func ErrorFromSerializedADKValue(value any) error {
	if err, ok := value.(error); ok {
		return classifySerializedADKError(err)
	}
	return ErrorFromSerializedADKText(fmt.Sprint(value))
}
