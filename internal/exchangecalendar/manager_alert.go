package exchangecalendar

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

func (m *Manager) recordSuccess(sourceID string, snapshot marketcalendar.CalendarSnapshot) {
	if m == nil {
		return
	}
	now := m.currentTime()
	var alert *SourceAlert
	m.mu.Lock()
	status := m.statuses[sourceID]
	if status == nil {
		status = &SourceStatus{SourceID: sourceID}
		m.statuses[sourceID] = status
	}
	status.LastSuccessAt = now
	status.LastError = ""
	status.ConsecutiveFailures = 0
	status.NextRefreshAt = time.Time{}
	status.LastSnapshotFetchedAt = snapshot.FetchedAt
	alert = recordHealthyStateLocked(status, normalizeMarket(snapshot.MarketCode), now)
	m.mu.Unlock()
	m.emitAlert(alert)
}

func (m *Manager) recordOperationFailure(sourceID string, err error) {
	if m == nil {
		return
	}
	now := m.currentTime()
	m.mu.Lock()
	status := m.statuses[sourceID]
	if status == nil {
		status = &SourceStatus{SourceID: sourceID}
		m.statuses[sourceID] = status
	}
	status.LastFailureAt = now
	if err != nil {
		status.LastError = err.Error()
	}
	status.ConsecutiveFailures++
	backoffHours := min(status.ConsecutiveFailures, 24)
	status.NextRefreshAt = now.Add(time.Duration(backoffHours) * time.Hour)
	m.mu.Unlock()
}

func (m *Manager) recordSourceFailure(sourceID string, market string, err error, kind string) {
	if m == nil {
		return
	}
	now := m.currentTime()
	var alert *SourceAlert
	m.mu.Lock()
	status := m.statuses[sourceID]
	if status == nil {
		status = &SourceStatus{SourceID: sourceID}
		m.statuses[sourceID] = status
	}
	status.LastFailureAt = now
	if err != nil {
		status.LastError = err.Error()
	} else {
		status.LastError = ""
	}
	status.ConsecutiveFailures++
	backoffHours := min(status.ConsecutiveFailures, 24)
	status.NextRefreshAt = now.Add(time.Duration(backoffHours) * time.Hour)
	alert = recordUnhealthyStateLocked(status, normalizeMarket(market), now, sourceFailureAlert(status.SourceID, market, kind, err))
	m.mu.Unlock()
	m.emitAlert(alert)
}

func (m *Manager) recordProbeSuccess(sourceID string, market string, scheduleCount int) {
	if m == nil {
		return
	}
	now := m.currentTime()
	var alert *SourceAlert
	m.mu.Lock()
	status := m.statuses[sourceID]
	if status == nil {
		status = &SourceStatus{SourceID: sourceID}
		m.statuses[sourceID] = status
	}
	status.LastProbeAt = now
	status.LastProbeSuccessAt = status.LastProbeAt
	status.LastProbeStatus = "healthy"
	status.LastProbeError = ""
	status.LastProbeMarket = normalizeMarket(market)
	status.LastProbeSchedules = scheduleCount
	alert = recordHealthyStateLocked(status, normalizeMarket(market), now)
	m.mu.Unlock()
	m.emitAlert(alert)
}

func (m *Manager) recordProbeFailure(sourceID string, market string, err error, kind string) {
	if m == nil {
		return
	}
	now := m.currentTime()
	var alert *SourceAlert
	m.mu.Lock()
	status := m.statuses[sourceID]
	if status == nil {
		status = &SourceStatus{SourceID: sourceID}
		m.statuses[sourceID] = status
	}
	status.LastProbeAt = now
	status.LastProbeFailureAt = status.LastProbeAt
	status.LastProbeStatus = "unhealthy"
	status.LastProbeMarket = normalizeMarket(market)
	status.LastProbeSchedules = 0
	if err != nil {
		status.LastProbeError = err.Error()
	} else {
		status.LastProbeError = ""
	}
	alert = recordUnhealthyStateLocked(status, normalizeMarket(market), now, sourceFailureAlert(status.SourceID, market, kind, err))
	m.mu.Unlock()
	m.emitAlert(alert)
}

func (m *Manager) emitAlert(alert *SourceAlert) {
	if m == nil || m.alertSink == nil || alert == nil {
		return
	}
	m.alertSink(*alert)
}

func recordHealthyStateLocked(status *SourceStatus, market string, now time.Time) *SourceAlert {
	if status == nil {
		return nil
	}
	previousState := status.HealthState
	previousFingerprint := status.HealthFingerprint
	status.HealthState = "healthy"
	status.HealthFingerprint = ""
	status.LastError = ""
	status.LastProbeError = ""
	status.ConsecutiveFailures = 0
	status.NextRefreshAt = time.Time{}
	if previousState != "unhealthy" {
		return nil
	}
	alert := &SourceAlert{
		SourceID:    status.SourceID,
		Market:      normalizeMarket(market),
		Level:       "success",
		Kind:        "recovered",
		Title:       "交易所日历源已恢复",
		Message:     fmt.Sprintf("%s 市场日历源 %s 已恢复正常解析。", normalizeMarket(market), status.SourceID),
		Fingerprint: previousFingerprint,
	}
	status.LastAlertAt = now
	status.LastAlertStatus = "recovered"
	status.LastAlertFingerprint = previousFingerprint
	return alert
}

func recordUnhealthyStateLocked(status *SourceStatus, market string, now time.Time, alert SourceAlert) *SourceAlert {
	if status == nil {
		return nil
	}
	fingerprint := strings.TrimSpace(alert.Fingerprint)
	if fingerprint == "" {
		fingerprint = fmt.Sprintf("%s|%s|unknown", status.SourceID, normalizeMarket(market))
		alert.Fingerprint = fingerprint
	}
	shouldNotify := status.HealthState != "unhealthy" || status.HealthFingerprint != fingerprint
	status.HealthState = "unhealthy"
	status.HealthFingerprint = fingerprint
	if !shouldNotify {
		return nil
	}
	alert.SourceID = status.SourceID
	alert.Market = normalizeMarket(market)
	status.LastAlertAt = now
	status.LastAlertStatus = "triggered"
	status.LastAlertFingerprint = fingerprint
	return &alert
}

func sourceFailureAlert(sourceID string, market string, kind string, err error) SourceAlert {
	normalizedMarket := normalizeMarket(market)
	message := ""
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	switch kind {
	case "structure_changed":
		return SourceAlert{
			SourceID:    strings.TrimSpace(sourceID),
			Market:      normalizedMarket,
			Level:       "error",
			Kind:        "structure_changed",
			Title:       "交易所日历源解析异常",
			Message:     fmt.Sprintf("%s 市场日历源 %s 抓取成功但未解析到有效交易日，可能是官网结构发生变化。系统将继续回退到内置日历。", normalizedMarket, sourceID),
			Fingerprint: sourceAlertFingerprint(sourceID, normalizedMarket, "structure_changed", err),
		}
	default:
		return SourceAlert{
			SourceID:    strings.TrimSpace(sourceID),
			Market:      normalizedMarket,
			Level:       "warn",
			Kind:        "fetch_failed",
			Title:       "交易所日历源抓取失败",
			Message:     fmt.Sprintf("%s 市场日历源 %s 抓取失败：%s。系统将继续回退到内置日历。", normalizedMarket, sourceID, defaultAlertDetail(message)),
			Fingerprint: sourceAlertFingerprint(sourceID, normalizedMarket, "fetch_failed", err),
		}
	}
}

func sourceAlertFingerprint(sourceID string, market string, kind string, err error) string {
	return fmt.Sprintf("%s|%s|%s|%s", strings.TrimSpace(sourceID), normalizeMarket(market), strings.TrimSpace(kind), sourceAlertFingerprintDetail(kind, err))
}

func sourceAlertFingerprintDetail(kind string, err error) string {
	if strings.TrimSpace(kind) == "structure_changed" {
		return "structure_changed"
	}
	if err == nil {
		return "unknown_error"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "network_timeout_or_cancelled"
	}
	detail := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(detail, "context canceled"),
		strings.Contains(detail, "context deadline exceeded"),
		strings.Contains(detail, "client.timeout exceeded"):
		return "network_timeout_or_cancelled"
	default:
		return detail
	}
}

func defaultAlertDetail(detail string) string {
	if strings.TrimSpace(detail) == "" {
		return "unknown error"
	}
	return strings.TrimSpace(detail)
}
