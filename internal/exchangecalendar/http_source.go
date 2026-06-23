package exchangecalendar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

type ParseFunc func(market string, body []byte, from time.Time, to time.Time) ([]marketcalendar.TradingDaySchedule, error)
type ValidateFunc func(market string, schedules []marketcalendar.TradingDaySchedule, from time.Time, to time.Time) error

type HTTPCalendarSource struct {
	id        string
	kind      string
	authority string
	markets   []string
	url       string
	client    *http.Client
	parse     ParseFunc
	validate  ValidateFunc
	validFor  time.Duration
}

func (s *HTTPCalendarSource) ID() string        { return s.id }
func (s *HTTPCalendarSource) Kind() string      { return s.kind }
func (s *HTTPCalendarSource) Markets() []string { return append([]string(nil), s.markets...) }
func (s *HTTPCalendarSource) Authority() string { return s.authority }

func (s *HTTPCalendarSource) Fetch(ctx context.Context, market string, from time.Time, to time.Time) (marketcalendar.CalendarSnapshot, error) {
	if s == nil {
		return marketcalendar.CalendarSnapshot{}, fmt.Errorf("http source is nil")
	}
	client := s.client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return marketcalendar.CalendarSnapshot{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return marketcalendar.CalendarSnapshot{}, err
	}
	defer func() { jftradeLogError(response.Body.Close()) }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return marketcalendar.CalendarSnapshot{}, fmt.Errorf("%s returned status %d", s.id, response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return marketcalendar.CalendarSnapshot{}, err
	}
	schedules := []marketcalendar.TradingDaySchedule{}
	if s.parse != nil {
		schedules, err = s.parse(market, body, from, to)
		if err != nil {
			return marketcalendar.CalendarSnapshot{}, err
		}
	}
	if s.validate != nil {
		if err := s.validate(market, schedules, from, to); err != nil {
			return marketcalendar.CalendarSnapshot{}, err
		}
	}
	sum := sha256.Sum256(body)
	fetchedAt := time.Now().UTC()
	return marketcalendar.CalendarSnapshot{
		MarketCode: normalizeMarket(market),
		SourceID:   s.id,
		From:       from,
		To:         to,
		Schedules:  schedules,
		FetchedAt:  fetchedAt,
		ValidUntil: fetchedAt.Add(s.validFor),
		Checksum:   hex.EncodeToString(sum[:]),
	}, nil
}

func (s *HTTPCalendarSource) ValidateSnapshot(market string, schedules []marketcalendar.TradingDaySchedule, from time.Time, to time.Time) error {
	if s == nil || s.validate == nil {
		return nil
	}
	return s.validate(market, schedules, from, to)
}

func DefaultRegistry(client *http.Client) *SourceRegistry {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	registry := NewSourceRegistry()
	registry.Register(&HTTPCalendarSource{
		id:        "nyse_official",
		kind:      "official_html",
		authority: "NYSE",
		markets:   []string{"US"},
		url:       "https://www.nyse.com/trade/hours-calendars",
		client:    client,
		parse:     nyseHolidayScheduleParser(),
		validate:  minimumAnchorYearSchedulesValidator(8),
		validFor:  14 * 24 * time.Hour,
	})
	registry.Register(&HTTPCalendarSource{
		id:        "nasdaq_verifier",
		kind:      "official_html",
		authority: "Nasdaq",
		markets:   []string{"US"},
		url:       "https://www.nasdaq.com/market-activity/stock-market-holiday-schedule",
		client:    client,
		parse:     defaultHolidayOverrideParser("US"),
		validate:  minimumAnchorYearSchedulesValidator(8),
		validFor:  14 * 24 * time.Hour,
	})
	registry.Register(&HTTPCalendarSource{
		id:        "hk_gov_1823_ical",
		kind:      "official_ical",
		authority: "GovHK 1823",
		markets:   []string{"HK"},
		url:       "https://www.1823.gov.hk/common/ical/en.ics",
		client:    client,
		parse:     hongKongHolidayICalParser(),
		validate:  minimumAnchorYearSchedulesValidator(8),
		validFor:  30 * 24 * time.Hour,
	})
	registry.Register(&HTTPCalendarSource{
		id:        "mainland_official_notice",
		kind:      "official_html",
		authority: "Shanghai Stock Exchange",
		markets:   []string{"CN", "SH", "SZ"},
		url:       "https://english.sse.com.cn/start/trading/schedule/",
		client:    client,
		parse:     sseTradingScheduleParser(),
		validate:  minimumAnchorYearSchedulesValidator(8),
		validFor:  30 * 24 * time.Hour,
	})
	return registry
}

func defaultHolidayOverrideParser(defaultMarket string) ParseFunc {
	return func(market string, body []byte, from time.Time, to time.Time) ([]marketcalendar.TradingDaySchedule, error) {
		targetMarket, template, builtin, ok := resolveParseTemplate(market, defaultMarket)
		if !ok {
			return nil, nil
		}

		lines := extractTextLines(string(body))
		schedules := make([]marketcalendar.TradingDaySchedule, 0)
		seen := map[string]struct{}{}
		for _, line := range lines {
			status, reason, ok := classifyHolidayLine(line)
			if !ok {
				continue
			}
			date, ok := parseLineDate(line, template, from, to)
			if !ok {
				continue
			}
			switch status {
			case marketcalendar.TradingDayClosed:
				schedules = appendSchedule(schedules, seen, marketcalendar.TradingDaySchedule{
					MarketCode: targetMarket,
					Date:       marketcalendar.DayStart(template, date),
					Status:     marketcalendar.TradingDayClosed,
					Reason:     reason,
					SourceID:   "",
				})
			case marketcalendar.TradingDayEarlyClose:
				if builtinSchedule, ok := builtin.Schedule(targetMarket, date); ok && builtinSchedule.Status == marketcalendar.TradingDayEarlyClose {
					builtinSchedule.Reason = reason
					builtinSchedule.SourceID = ""
					schedules = appendSchedule(schedules, seen, builtinSchedule)
				}
			}
		}
		sortSchedulesByDate(schedules)
		return schedules, nil
	}
}

func nyseHolidayScheduleParser() ParseFunc {
	return func(market string, body []byte, from time.Time, to time.Time) ([]marketcalendar.TradingDaySchedule, error) {
		targetMarket, template, builtin, ok := resolveParseTemplate(market, "US")
		if !ok {
			return nil, nil
		}

		rows := extractHTMLTableRows(string(body))
		years, headerIndex := extractNYSEHeaderYears(rows)
		schedules := make([]marketcalendar.TradingDaySchedule, 0)
		seen := map[string]struct{}{}

		if len(years) > 0 {
			for _, row := range rows[headerIndex+1:] {
				if len(row) < len(years)+1 {
					continue
				}
				holidayName := normalizeHTMLText(stripHTML(row[0], " "))
				if holidayName == "" {
					continue
				}
				for idx, year := range years {
					cell := normalizeHTMLText(stripHTML(row[idx+1], " "))
					date, ok := parseMonthDayCellWithYear(cell, year, template)
					if !ok || !dateWithinFetchRange(date, from, to, template) {
						continue
					}
					schedules = appendSchedule(schedules, seen, marketcalendar.TradingDaySchedule{
						MarketCode: targetMarket,
						Date:       marketcalendar.DayStart(template, date),
						Status:     marketcalendar.TradingDayClosed,
						Reason:     normalizedReason(holidayName),
						Observed:   strings.Contains(strings.ToLower(cell), "observed"),
					})
				}
			}
		}

		for _, line := range extractTextLines(string(body)) {
			status, reason, ok := classifyHolidayLine(line)
			if !ok || status != marketcalendar.TradingDayEarlyClose {
				continue
			}
			for _, date := range extractLineDates(line, template, from, to) {
				if builtinSchedule, ok := builtin.Schedule(targetMarket, date); ok && builtinSchedule.Status == marketcalendar.TradingDayEarlyClose {
					builtinSchedule.Reason = reason
					builtinSchedule.SourceID = ""
					schedules = appendSchedule(schedules, seen, builtinSchedule)
				}
			}
		}

		sortSchedulesByDate(schedules)
		return schedules, nil
	}
}

func hongKongHolidayICalParser() ParseFunc {
	return func(market string, body []byte, from time.Time, to time.Time) ([]marketcalendar.TradingDaySchedule, error) {
		targetMarket, template, _, ok := resolveParseTemplate(market, "HK")
		if !ok {
			return nil, nil
		}

		lines := unfoldICalLines(string(body))
		schedules := make([]marketcalendar.TradingDaySchedule, 0)
		seen := map[string]struct{}{}

		inEvent := false
		eventDate := time.Time{}
		eventSummary := ""
		flushEvent := func() {
			if !inEvent || eventDate.IsZero() || !dateWithinFetchRange(eventDate, from, to, template) {
				return
			}
			schedules = appendSchedule(schedules, seen, marketcalendar.TradingDaySchedule{
				MarketCode: targetMarket,
				Date:       marketcalendar.DayStart(template, eventDate),
				Status:     marketcalendar.TradingDayClosed,
				Reason:     normalizedReason(eventSummary),
			})
		}

		for _, line := range lines {
			switch {
			case line == "BEGIN:VEVENT":
				inEvent = true
				eventDate = time.Time{}
				eventSummary = ""
			case line == "END:VEVENT":
				flushEvent()
				inEvent = false
				eventDate = time.Time{}
				eventSummary = ""
			case !inEvent:
				continue
			case strings.HasPrefix(line, "DTSTART"):
				eventDate = parseICalDateValue(line, template)
			case strings.HasPrefix(line, "SUMMARY"):
				eventSummary = decodeICalText(fieldValue(line))
			}
		}

		sortSchedulesByDate(schedules)
		return schedules, nil
	}
}

func sseTradingScheduleParser() ParseFunc {
	return func(market string, body []byte, from time.Time, to time.Time) ([]marketcalendar.TradingDaySchedule, error) {
		targetMarket, template, _, ok := resolveParseTemplate(market, "CN")
		if !ok {
			return nil, nil
		}

		lines := extractTextLines(string(body))
		schedules := make([]marketcalendar.TradingDaySchedule, 0)
		seen := map[string]struct{}{}
		currentYear := 0

		for _, line := range lines {
			if year, ok := parseStandaloneYear(line); ok {
				currentYear = year
				continue
			}
			if currentYear == 0 {
				continue
			}
			if strings.Contains(strings.ToLower(line), "trading hours") {
				break
			}
			if !containsMonthName(line) {
				continue
			}
			for _, span := range extractSSEDateSpans(line, currentYear, template) {
				start := marketcalendar.DayStart(template, span[0])
				end := marketcalendar.DayStart(template, span[1])
				if end.Before(start) {
					continue
				}
				for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
					if !dateWithinFetchRange(day, from, to, template) {
						continue
					}
					schedules = appendSchedule(schedules, seen, marketcalendar.TradingDaySchedule{
						MarketCode: targetMarket,
						Date:       day,
						Status:     marketcalendar.TradingDayClosed,
						Reason:     normalizedReason(line),
					})
				}
			}
		}

		sortSchedulesByDate(schedules)
		return schedules, nil
	}
}

func extractTextLines(body string) []string {
	lines := make([]string, 0)
	seen := map[string]struct{}{}
	appendLine := func(line string) {
		trimmed := normalizeHTMLText(line)
		if trimmed == "" {
			return
		}
		if _, ok := seen[trimmed]; ok {
			return
		}
		seen[trimmed] = struct{}{}
		lines = append(lines, trimmed)
	}

	rowPattern := regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	rowMatches := rowPattern.FindAllStringSubmatch(body, -1)
	for _, match := range rowMatches {
		if len(match) < 2 {
			continue
		}
		appendLine(stripHTML(match[1], " "))
	}

	blockPattern := regexp.MustCompile(`(?is)<(li|p|div|section|article)[^>]*>(.*?)</(li|p|div|section|article)>`)
	blockMatches := blockPattern.FindAllStringSubmatch(body, -1)
	for _, match := range blockMatches {
		if len(match) < 3 {
			continue
		}
		appendLine(stripHTML(match[2], " "))
	}

	tagPattern := regexp.MustCompile(`(?s)<[^>]+>`)
	cleaned := tagPattern.ReplaceAllString(body, "\n")
	for line := range strings.SplitSeq(cleaned, "\n") {
		appendLine(line)
	}
	return lines
}

func extractHTMLTableRows(body string) [][]string {
	rowPattern := regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	cellPattern := regexp.MustCompile(`(?is)<t[hd][^>]*>(.*?)</t[hd]>`)
	matches := rowPattern.FindAllStringSubmatch(body, -1)
	rows := make([][]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		cellMatches := cellPattern.FindAllStringSubmatch(match[1], -1)
		if len(cellMatches) == 0 {
			continue
		}
		row := make([]string, 0, len(cellMatches))
		for _, cell := range cellMatches {
			if len(cell) < 2 {
				continue
			}
			row = append(row, cell[1])
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}
	return rows
}

func unfoldICalLines(body string) []string {
	rawLines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, raw := range rawLines {
		line := strings.TrimRight(raw, "\r")
		if len(lines) > 0 && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			lines[len(lines)-1] += strings.TrimLeft(line, " \t")
			continue
		}
		lines = append(lines, strings.TrimSpace(line))
	}
	return lines
}

func fieldValue(line string) string {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func parseICalDateValue(line string, template marketcalendar.MarketTemplate) time.Time {
	value := fieldValue(line)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{"20060102", "20060102T150405Z", "20060102T150405"} {
		parsed, err := time.ParseInLocation(layout, value, marketcalendar.LoadLocation(template))
		if err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func decodeICalText(value string) string {
	replacer := strings.NewReplacer(`\n`, " ", `\N`, " ", `\,`, ",", `\;`, ";", `\\`, `\`)
	return normalizeHTMLText(replacer.Replace(value))
}

func classifyHolidayLine(line string) (marketcalendar.TradingDayStatus, string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(line))
	switch {
	case strings.Contains(normalized, "closed"), strings.Contains(normalized, "休市"):
		return marketcalendar.TradingDayClosed, normalizedReason(normalized), true
	case strings.Contains(normalized, "early close"),
		strings.Contains(normalized, "half day"),
		strings.Contains(normalized, "half-day"),
		strings.Contains(normalized, "提前收市"):
		return marketcalendar.TradingDayEarlyClose, normalizedReason(normalized), true
	case strings.Contains(normalized, "1:00 p.m.") || strings.Contains(normalized, "1:00 pm"):
		return marketcalendar.TradingDayEarlyClose, normalizedReason(normalized), true
	default:
		return marketcalendar.TradingDayUnknown, "", false
	}
}

func normalizedReason(line string) string {
	replacer := strings.NewReplacer(" ", "_", ",", "", ".", "", "(", "", ")", "", "/", "_")
	return replacer.Replace(strings.ToLower(strings.TrimSpace(line)))
}

func parseLineDate(line string, template marketcalendar.MarketTemplate, from time.Time, to time.Time) (time.Time, bool) {
	dates := extractLineDates(line, template, from, to)
	if len(dates) == 0 {
		return time.Time{}, false
	}
	return dates[0], true
}

func extractLineDates(line string, template marketcalendar.MarketTemplate, from time.Time, to time.Time) []time.Time {
	layouts := []string{
		"January 2, 2006",
		"Jan 2, 2006",
		"2 January 2006",
		"2006-01-02",
		"2006/01/02",
		"2006.01.02",
		"2006年1月2日",
	}
	datePattern := regexp.MustCompile(`(?i)([A-Z][a-z]+ \d{1,2}, \d{4}|[A-Z][a-z]{2} \d{1,2}, \d{4}|\d{1,2} [A-Z][a-z]+ \d{4}|\d{4}-\d{2}-\d{2}|\d{4}/\d{2}/\d{2}|\d{4}\.\d{2}\.\d{2}|\d{4}年\d{1,2}月\d{1,2}日)`)
	matches := datePattern.FindAllString(line, -1)
	dates := make([]time.Time, 0, len(matches))
	for _, match := range matches {
		for _, layout := range layouts {
			parsed, err := time.ParseInLocation(layout, match, marketcalendar.LoadLocation(template))
			if err != nil {
				continue
			}
			if !dateWithinFetchRange(parsed, from, to, template) {
				continue
			}
			dates = append(dates, parsed)
			break
		}
	}
	return dates
}

func stripHTML(body string, separator string) string {
	tagPattern := regexp.MustCompile(`(?s)<[^>]+>`)
	return tagPattern.ReplaceAllString(body, separator)
}

func normalizeHTMLText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func resolveParseTemplate(market string, defaultMarket string) (string, marketcalendar.MarketTemplate, *marketcalendar.BuiltinResolver, bool) {
	targetMarket := normalizeMarket(market)
	if targetMarket == "" {
		targetMarket = normalizeMarket(defaultMarket)
	}
	builtin := marketcalendar.NewBuiltinResolver()
	template, ok := builtin.Template(targetMarket)
	if !ok && (targetMarket == "SH" || targetMarket == "SZ") {
		template, ok = builtin.Template("CN")
	}
	return targetMarket, template, builtin, ok
}

func appendSchedule(schedules []marketcalendar.TradingDaySchedule, seen map[string]struct{}, schedule marketcalendar.TradingDaySchedule) []marketcalendar.TradingDaySchedule {
	dayKey := schedule.Date.Format("2006-01-02")
	if _, ok := seen[dayKey]; ok {
		return schedules
	}
	seen[dayKey] = struct{}{}
	return append(schedules, schedule)
}

func extractNYSEHeaderYears(rows [][]string) ([]int, int) {
	for rowIndex, row := range rows {
		if len(row) < 2 {
			continue
		}
		if normalizeHTMLText(stripHTML(row[0], " ")) != "Holiday" {
			continue
		}
		years := make([]int, 0, len(row)-1)
		for _, cell := range row[1:] {
			year, err := strconv.Atoi(normalizeHTMLText(stripHTML(cell, " ")))
			if err != nil {
				years = nil
				break
			}
			years = append(years, year)
		}
		if len(years) > 0 {
			return years, rowIndex
		}
	}
	return nil, -1
}

func parseMonthDayCellWithYear(cell string, year int, template marketcalendar.MarketTemplate) (time.Time, bool) {
	monthDayPattern := regexp.MustCompile(`(?i)(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2})`)
	match := monthDayPattern.FindStringSubmatch(cell)
	if len(match) != 3 {
		return time.Time{}, false
	}
	parsed, err := time.ParseInLocation("January 2 2006", fmt.Sprintf("%s %s %d", match[1], match[2], year), marketcalendar.LoadLocation(template))
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func parseStandaloneYear(line string) (int, bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(line, "##"))
	yearPattern := regexp.MustCompile(`^\d{4}$`)
	if !yearPattern.MatchString(trimmed) {
		return 0, false
	}
	year, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, false
	}
	return year, true
}

func containsMonthName(line string) bool {
	lower := strings.ToLower(line)
	for _, month := range []string{
		"january", "february", "march", "april", "may", "june",
		"july", "august", "september", "october", "november", "december",
	} {
		if strings.Contains(lower, month) {
			return true
		}
	}
	return false
}

func extractSSEDateSpans(line string, year int, template marketcalendar.MarketTemplate) [][2]time.Time {
	truncated := line
	if marker := strings.Index(strings.ToLower(truncated), ", plus "); marker >= 0 {
		truncated = truncated[:marker]
	}
	rangePattern := regexp.MustCompile(`(?i)(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2})(?:,\s*(\d{4}))?(?:\s*\([^)]*\))?(?:\s*-\s*(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2})(?:,\s*(\d{4}))?(?:\s*\([^)]*\))?)?`)
	matches := rangePattern.FindAllStringSubmatch(truncated, -1)
	spans := make([][2]time.Time, 0, len(matches))
	for _, match := range matches {
		if len(match) < 6 {
			continue
		}
		startYear := year
		if strings.TrimSpace(match[3]) != "" {
			if parsedYear, err := strconv.Atoi(strings.TrimSpace(match[3])); err == nil {
				startYear = parsedYear
			}
		}
		start, ok := parseMonthDayWithOptionalYear(match[1], match[2], startYear, template)
		if !ok {
			continue
		}
		end := start
		if strings.TrimSpace(match[4]) != "" && strings.TrimSpace(match[5]) != "" {
			endYear := startYear
			if strings.TrimSpace(match[6]) != "" {
				if parsedYear, err := strconv.Atoi(strings.TrimSpace(match[6])); err == nil {
					endYear = parsedYear
				}
			}
			parsedEnd, ok := parseMonthDayWithOptionalYear(match[4], match[5], endYear, template)
			if !ok {
				continue
			}
			end = parsedEnd
		}
		spans = append(spans, [2]time.Time{start, end})
	}
	return spans
}

func parseMonthDayWithOptionalYear(month string, day string, year int, template marketcalendar.MarketTemplate) (time.Time, bool) {
	parsed, err := time.ParseInLocation("January 2 2006", fmt.Sprintf("%s %s %d", month, day, year), marketcalendar.LoadLocation(template))
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func dateWithinFetchRange(date time.Time, from time.Time, to time.Time, template marketcalendar.MarketTemplate) bool {
	day := marketcalendar.DayStart(template, date)
	if !from.IsZero() {
		fromDay := marketcalendar.DayStart(template, from)
		if day.Before(fromDay) {
			return false
		}
	}
	if !to.IsZero() {
		toDay := marketcalendar.DayStart(template, to)
		if day.After(toDay) {
			return false
		}
	}
	return true
}

func sortSchedulesByDate(schedules []marketcalendar.TradingDaySchedule) {
	sort.SliceStable(schedules, func(i, j int) bool {
		return schedules[i].Date.Before(schedules[j].Date)
	})
}

func minimumAnchorYearSchedulesValidator(minimumPerYear int) ValidateFunc {
	return func(market string, schedules []marketcalendar.TradingDaySchedule, from time.Time, to time.Time) error {
		if minimumPerYear <= 0 {
			return nil
		}
		anchorYear := from.Year()
		if anchorYear <= 0 {
			if len(schedules) == 0 {
				return fmt.Errorf("%s parsed no schedules and no anchor year is available", normalizeMarket(market))
			}
			anchorYear = schedules[0].Date.Year()
		}
		count := 0
		for _, schedule := range schedules {
			if schedule.Date.Year() == anchorYear {
				count++
			}
		}
		if count < minimumPerYear {
			return fmt.Errorf("%s parsed too few anchor-year schedules for %d: got %d, want at least %d", normalizeMarket(market), anchorYear, count, minimumPerYear)
		}
		return nil
	}
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
