package pine

import (
	"regexp"
	"sort"
	"strings"
)

func replaceMathNamespace(expression string) string {
	for _, name := range []string{"abs", "min", "max", "avg", "round", "round_to_mintick", "floor", "ceil", "sqrt", "pow", "log", "sign"} {
		expression = strings.ReplaceAll(expression, "math."+name, name)
	}
	return expression
}

func replacePineNamespaceConstants(expression string) string {
	replacements := map[string]string{
		"barstate.isfirst":       "barstate_isfirst",
		"barstate.isnew":         "barstate_isnew",
		"barstate.isconfirmed":   "barstate_isconfirmed",
		"barstate.ishistory":     "barstate_ishistory",
		"barstate.isrealtime":    "barstate_isrealtime",
		"barstate.islast":        "barstate_islast",
		"session.ismarket":       "session_ismarket",
		"session.ispremarket":    "session_ispremarket",
		"session.ispostmarket":   "session_ispostmarket",
		"dayofweek.sunday":       "1",
		"dayofweek.monday":       "2",
		"dayofweek.tuesday":      "3",
		"dayofweek.wednesday":    "4",
		"dayofweek.thursday":     "5",
		"dayofweek.friday":       "6",
		"dayofweek.saturday":     "7",
		"month.january":          "1",
		"month.february":         "2",
		"month.march":            "3",
		"month.april":            "4",
		"month.may":              "5",
		"month.june":             "6",
		"month.july":             "7",
		"month.august":           "8",
		"month.september":        "9",
		"month.october":          "10",
		"month.november":         "11",
		"month.december":         "12",
		"color.black":            "\"#000000\"",
		"color.white":            "\"#ffffff\"",
		"color.red":              "\"#ff5252\"",
		"color.green":            "\"#4caf50\"",
		"color.blue":             "\"#2196f3\"",
		"color.yellow":           "\"#ffeb3b\"",
		"color.orange":           "\"#ff9800\"",
		"color.purple":           "\"#9c27b0\"",
		"color.gray":             "\"#787b86\"",
		"color.grey":             "\"#787b86\"",
		"color.aqua":             "\"#00bcd4\"",
		"color.lime":             "\"#00e676\"",
		"color.maroon":           "\"#880e4f\"",
		"color.navy":             "\"#311b92\"",
		"color.olive":            "\"#808000\"",
		"color.silver":           "\"#b2b5be\"",
		"color.teal":             "\"#00897b\"",
		"color.fuchsia":          "\"#e040fb\"",
		"format.mintick":         "\"mintick\"",
		"format.percent":         "\"percent\"",
		"format.volume":          "\"volume\"",
		"barmerge.gaps_off":      "\"gaps_off\"",
		"barmerge.lookahead_off": "\"lookahead_off\"",
	}
	keys := make([]string, 0, len(replacements))
	for key := range replacements {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		return len(keys[left]) > len(keys[right])
	})
	result := expression
	for _, key := range keys {
		result = regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(key)+`\b`).ReplaceAllString(result, replacements[key])
	}
	return result
}
