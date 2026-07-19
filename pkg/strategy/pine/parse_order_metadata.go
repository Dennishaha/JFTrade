package pine

import (
	"fmt"
	"strings"
)

func rejectConflictingQuantityArgs(lineNumber int, functionName string, args []string) error {
	if hasNamedArg(args, "qty") && hasNamedArg(args, "qty_percent") {
		return fmt.Errorf("pine line %d: %s supports qty or qty_percent, not both", lineNumber, functionName)
	}
	return nil
}

func rejectUnsupportedNamedArgs(lineNumber int, functionName string, args []string, allowedNames ...string) error {
	allowed := make(map[string]struct{}, len(allowedNames))
	for _, name := range allowedNames {
		allowed[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	for _, arg := range args {
		key, _, ok := splitNamedArg(arg)
		if !ok {
			continue
		}
		if _, exists := allowed[strings.ToLower(strings.TrimSpace(key))]; exists {
			continue
		}
		return fmt.Errorf("pine line %d: %s argument %s is not supported by JFTrade", lineNumber, functionName, key)
	}
	return nil
}

func rejectUnsupportedOrderArgs(lineNumber int, functionName string, args []string) error {
	for _, name := range []string{"oca_name", "oca_type"} {
		if hasNamedArg(args, name) {
			return fmt.Errorf("pine line %d: %s argument %s is parsed but not executable by JFTrade yet", lineNumber, functionName, name)
		}
	}
	return nil
}

func pineOrderMetadata(lineNumber int, functionName string, args []string, allowImmediate bool) (string, string, bool, error) {
	comment := ""
	if raw, ok := namedArgValue(args, "comment"); ok {
		comment = unquote(strings.TrimSpace(raw))
	}
	alertMessage := ""
	if raw, ok := namedArgValue(args, "alert_message"); ok {
		alertMessage = unquote(strings.TrimSpace(raw))
	}
	disableAlert := false
	if raw, ok := namedArgValue(args, "disable_alert"); ok {
		value, valid := parseBoolConstant(raw)
		if !valid {
			return "", "", false, fmt.Errorf("pine line %d: %s disable_alert must be true or false", lineNumber, functionName)
		}
		disableAlert = value
	}
	if hasNamedArg(args, "immediately") && !allowImmediate {
		return "", "", false, fmt.Errorf("pine line %d: %s does not support immediately", lineNumber, functionName)
	}
	return comment, alertMessage, disableAlert, nil
}

func pineCloseMetadata(lineNumber int, functionName string, args []string) (string, string, bool, bool, error) {
	comment, alertMessage, disableAlert, err := pineOrderMetadata(lineNumber, functionName, args, true)
	if err != nil {
		return "", "", false, false, err
	}
	immediate := false
	if raw, ok := namedArgValue(args, "immediately"); ok {
		value, valid := parseBoolConstant(raw)
		if !valid {
			return "", "", false, false, fmt.Errorf("pine line %d: %s immediately must be true or false", lineNumber, functionName)
		}
		immediate = value
	}
	return comment, alertMessage, disableAlert, immediate, nil
}

func pineCloseAllMetadata(lineNumber int, args []string) (string, string, bool, bool, error) {
	comment, alertMessage, disableAlert, immediate, err := pineCloseMetadata(lineNumber, "strategy.close_all", args)
	if err != nil {
		return "", "", false, false, err
	}
	if _, ok := namedArgValue(args, "immediately"); !ok && len(args) > 0 && !strings.Contains(args[0], "=") {
		value, valid := parseBoolConstant(args[0])
		if !valid {
			return "", "", false, false, fmt.Errorf("pine line %d: strategy.close_all immediately must be true or false", lineNumber)
		}
		immediate = value
	}
	if _, ok := namedArgValue(args, "comment"); !ok && len(args) > 1 && !strings.Contains(args[1], "=") {
		comment = unquote(strings.TrimSpace(args[1]))
	}
	if _, ok := namedArgValue(args, "alert_message"); !ok && len(args) > 2 && !strings.Contains(args[2], "=") {
		alertMessage = unquote(strings.TrimSpace(args[2]))
	}
	if _, ok := namedArgValue(args, "disable_alert"); !ok && len(args) > 3 && !strings.Contains(args[3], "=") {
		value, valid := parseBoolConstant(args[3])
		if !valid {
			return "", "", false, false, fmt.Errorf("pine line %d: strategy.close_all disable_alert must be true or false", lineNumber)
		}
		disableAlert = value
	}
	for index := 4; index < len(args); index++ {
		if !strings.Contains(args[index], "=") {
			return "", "", false, false, fmt.Errorf("pine line %d: strategy.close_all supports positional immediately, comment, alert_message, and disable_alert only", lineNumber)
		}
	}
	return comment, alertMessage, disableAlert, immediate, nil
}

type pineExitMetadataFields struct {
	comment         string
	commentProfit   string
	commentLoss     string
	commentTrailing string
	alertMessage    string
	alertProfit     string
	alertLoss       string
	alertTrailing   string
	disableAlert    bool
}

func pineExitMetadata(lineNumber int, args []string) (pineExitMetadataFields, error) {
	comment, alertMessage, disableAlert, err := pineOrderMetadata(lineNumber, "strategy.exit", args, false)
	if err != nil {
		return pineExitMetadataFields{}, err
	}
	commentProfit := ""
	if raw, ok := namedArgValue(args, "comment_profit"); ok {
		commentProfit = unquote(strings.TrimSpace(raw))
	}
	commentLoss := ""
	if raw, ok := namedArgValue(args, "comment_loss"); ok {
		commentLoss = unquote(strings.TrimSpace(raw))
	}
	commentTrailing := ""
	if raw, ok := namedArgValue(args, "comment_trailing"); ok {
		commentTrailing = unquote(strings.TrimSpace(raw))
	}
	alertProfit := ""
	if raw, ok := namedArgValue(args, "alert_profit"); ok {
		alertProfit = unquote(strings.TrimSpace(raw))
	}
	alertLoss := ""
	if raw, ok := namedArgValue(args, "alert_loss"); ok {
		alertLoss = unquote(strings.TrimSpace(raw))
	}
	alertTrailing := ""
	if raw, ok := namedArgValue(args, "alert_trailing"); ok {
		alertTrailing = unquote(strings.TrimSpace(raw))
	}
	return pineExitMetadataFields{
		comment:         comment,
		commentProfit:   commentProfit,
		commentLoss:     commentLoss,
		commentTrailing: commentTrailing,
		alertMessage:    alertMessage,
		alertProfit:     alertProfit,
		alertLoss:       alertLoss,
		alertTrailing:   alertTrailing,
		disableAlert:    disableAlert,
	}, nil
}
