package adk

import (
	"errors"
	"fmt"
	"strings"

	adktool "google.golang.org/adk/v2/tool"
	adkworkflow "google.golang.org/adk/v2/workflow"
)

var (
	ErrInvalidTaskStatus = errors.New("invalid task status")
	ErrProviderInUse     = errors.New("provider is used by agent")

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
	if err, ok := value.(error); ok {
		return classifySerializedADKError(err)
	}
	return errorFromSerializedADKText(fmt.Sprint(value))
}

func errorFromSerializedADKText(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return classifySerializedADKError(errors.New(text))
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
		errUserGoalPauseRequested,
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
