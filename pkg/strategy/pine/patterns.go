package pine

import (
	"regexp"
	"strings"
)

var (
	strategyTitlePattern       = regexp.MustCompile(`(?i)^strategy\s*\(\s*("[^"]*"|'[^']*'|[^,\)]*)`)
	assignmentPattern          = regexp.MustCompile(`^(?:(var|varip|const)\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*(:=|=)\s*(.+)$`)
	objectFieldAssignPattern   = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)\s*(:=|=)\s*(.+)$`)
	typedAssignmentPattern     = regexp.MustCompile(`^(?:(var|varip|const)\s+)?((?i:array|map|matrix)\s*<[^>]+>|(?i:array|map|matrix))\s+([A-Za-z_][A-Za-z0-9_]*)\s*(:=|=)\s*(.+)$`)
	tupleAssignmentPattern     = regexp.MustCompile(`^\[\s*([A-Za-z_][A-Za-z0-9_]*)(?:\s*,\s*([A-Za-z_][A-Za-z0-9_]*))?(?:\s*,\s*([A-Za-z_][A-Za-z0-9_]*))?\s*\]\s*(:=|=)\s*(.+)$`)
	generalTuplePattern        = regexp.MustCompile(`^\[\s*([^\]]+)\s*\]\s*(:=|=)\s*(.+)$`)
	inputCallPattern           = regexp.MustCompile(`(?i)^input\.[A-Za-z_][A-Za-z0-9_]*\s*\(\s*([^,\)]+)`)
	equityQuantityPattern      = regexp.MustCompile(`(?i)^\(?\s*strategy\.equity\s*\*\s*([0-9]+(?:\.[0-9]+)?)\s*/\s*100\s*\)?\s*/\s*close$`)
	amountQuantityPattern      = regexp.MustCompile(`(?i)^\(?\s*([0-9]+(?:\.[0-9]+)?)\s*/\s*close\s*\)?$`)
	entryPolicyAnnotPattern    = regexp.MustCompile(`@entry_policy\s+(\S+)`)
	exitPricePattern           = regexp.MustCompile(`(?i)^close\s*\*\s*\(?\s*1\s*[+-]\s*([0-9]+(?:\.[0-9]+)?)\s*/\s*100\s*\)?$`)
	exitTrailPattern           = regexp.MustCompile(`(?i)^close\s*\*\s*([0-9]+(?:\.[0-9]+)?)\s*/\s*100$`)
	historyReferencePattern    = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?)\s*\[\s*([0-9]+)\s*\]`)
	objectHistoryMethodPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\[\s*([0-9]+)\s*\]\s*\.\s*([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	callHistoryPattern         = regexp.MustCompile(`\)\s*\[\s*[0-9]+\s*\]`)
	udfPattern                 = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)\s*=>\s*(.*)$`)
	forLoopPattern             = regexp.MustCompile(`(?i)^for\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.+?)\s+to\s+(.+?)(?:\s+by\s+(.+))?\s*$`)
	collectionForLoopPattern   = regexp.MustCompile(`(?i)^for\s+(?:\[([A-Za-z_][A-Za-z0-9_]*|_)\s*,\s*([A-Za-z_][A-Za-z0-9_]*|_)\]|([A-Za-z_][A-Za-z0-9_]*|_))\s+in\s+(.+)$`)
	identifierPattern          = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	memberPattern              = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?$`)
	numberPattern              = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?$`)
	taTRPattern                = regexp.MustCompile(`(?i)\bta\.tr\b`)
	taOBVPattern               = regexp.MustCompile(`(?i)\bta\.obv\b`)
)

func buildEntryPolicyCache(script string) map[int]string {
	cache := map[int]string{}
	normalized := strings.ReplaceAll(script, "\r\n", "\n")
	for lineNumMinus1, rawLine := range strings.Split(normalized, "\n") {
		trimmed := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(trimmed, "//") {
			continue
		}
		match := entryPolicyAnnotPattern.FindStringSubmatch(trimmed)
		if match == nil {
			continue
		}
		cache[lineNumMinus1+2] = strings.ToLower(strings.TrimSpace(match[1])) // +2 = current line + next line
	}
	return cache
}

func (s *parseState) readEntryPolicyForLine(lineNumber int) string {
	if policy, ok := s.entryPolicyCache[lineNumber]; ok {
		return policy
	}
	return "same_direction"
}
