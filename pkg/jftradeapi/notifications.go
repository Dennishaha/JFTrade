package jftradeapi

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	bbgo "github.com/c9s/bbgo/pkg/bbgo"
	bbgotypes "github.com/c9s/bbgo/pkg/types"
	"github.com/gorilla/websocket"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

const (
	maxLiveNotificationEvents = 200
	liveNotificationRetention = 24 * time.Hour
)

var (
	bbgoNotifierBridgeOnce  sync.Once
	bbgoNotifierBridgeMu    sync.RWMutex
	bbgoNotifierBridgeSeq   uint64
	bbgoNotifierBridgeSinks = map[uint64]func(liveNotification) *liveNotificationEvent{}
)

type liveNotification struct {
	At       string
	Level    string
	Title    string
	Message  string
	Source   string
	BrokerID string
	Category string
}

type liveNotificationEvent struct {
	Sequence   uint64
	RecordedAt time.Time
	At         string
	Level      string
	Title      string
	Message    string
	Source     string
	BrokerID   string
	Category   string
}

type forwardedBBGONotification struct {
	note liveNotification
}

func (notification forwardedBBGONotification) String() string {
	return notification.note.text()
}

type liveSocketBBGONotifier struct{}

func registerBBGONotificationSink(sink func(liveNotification) *liveNotificationEvent) uint64 {
	if sink == nil {
		return 0
	}
	bbgoNotifierBridgeOnce.Do(func() {
		bbgo.Notification.AddNotifier(liveSocketBBGONotifier{})
	})
	id := atomic.AddUint64(&bbgoNotifierBridgeSeq, 1)
	bbgoNotifierBridgeMu.Lock()
	bbgoNotifierBridgeSinks[id] = sink
	bbgoNotifierBridgeMu.Unlock()
	return id
}

func (liveSocketBBGONotifier) Notify(obj any, args ...any) {
	note := liveNotificationFromBBGONotify(obj, args...)
	if note == nil {
		return
	}
	dispatchBBGONotification(*note)
}

func (liveSocketBBGONotifier) Upload(file *bbgotypes.UploadFile) {
	if file == nil {
		return
	}
	note := liveNotification{
		At:       time.Now().UTC().Format(time.RFC3339Nano),
		Level:    "info",
		Title:    "BBGO 文件通知",
		Source:   "bbgo.notify",
		Category: "bbgo.upload",
	}
	if caption := strings.TrimSpace(file.Caption); caption != "" {
		note.Message = caption
	} else {
		note.Message = string(file.FileType)
	}
	dispatchBBGONotification(note)
}

func dispatchBBGONotification(note liveNotification) {
	bbgoNotifierBridgeMu.RLock()
	sinks := make([]func(liveNotification) *liveNotificationEvent, 0, len(bbgoNotifierBridgeSinks))
	for _, sink := range bbgoNotifierBridgeSinks {
		sinks = append(sinks, sink)
	}
	bbgoNotifierBridgeMu.RUnlock()
	for _, sink := range sinks {
		sink(note)
	}
}

func (s *Server) ensureLiveNotificationBridge(ctx context.Context) {
	exchange := s.futuExchange()
	go func() {
		bridgeCtx, cancel := context.WithTimeout(ctx, liveStreamConnectTimeout)
		defer cancel()
		_ = exchange.EnsureSystemNotifications(bridgeCtx)
	}()
}

func (s *Server) handleFutuSystemNotify(response *notifypb.Response) {
	note := liveNotificationFromFutuResponse(response)
	if note == nil {
		return
	}
	s.recordLiveNotification(*note)
	if shouldForwardNotificationToBBGO(*note) {
		bbgo.Notify(forwardedBBGONotification{note: *note})
	}
}

func liveNotificationFromBBGONotify(obj any, args ...any) *liveNotification {
	if obj == nil {
		return nil
	}
	if _, ok := obj.(forwardedBBGONotification); ok {
		return nil
	}

	note := liveNotification{
		At:       time.Now().UTC().Format(time.RFC3339Nano),
		Level:    "info",
		Title:    "BBGO 通知",
		Source:   "bbgo.notify",
		Category: "bbgo.notify",
	}

	switch value := obj.(type) {
	case error:
		note.Level = "error"
		note.Title = "BBGO 错误"
		note.Message = strings.TrimSpace(value.Error())
	default:
		text := strings.TrimSpace(formatBBGONotifyText(obj, args...))
		if text == "" {
			return nil
		}
		note.Level = inferBBGONotificationLevel(text)
		note.Message = text
	}

	if note.Message == "" {
		return nil
	}
	return &note
}

func formatBBGONotifyText(obj any, args ...any) string {
	switch value := obj.(type) {
	case string:
		if len(args) == 0 {
			return value
		}
		formatted := fmt.Sprintf(value, args...)
		if formatted != value {
			return formatted
		}
		return strings.TrimSpace(value + " " + joinNotifyArgs(args...))
	case fmt.Stringer:
		if len(args) == 0 {
			return value.String()
		}
		return strings.TrimSpace(value.String() + " " + joinNotifyArgs(args...))
	default:
		if len(args) == 0 {
			return fmt.Sprint(obj)
		}
		return strings.TrimSpace(fmt.Sprint(obj) + " " + joinNotifyArgs(args...))
	}
}

func joinNotifyArgs(args ...any) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, strings.TrimSpace(fmt.Sprint(arg)))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func inferBBGONotificationLevel(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "panic"),
		strings.Contains(lower, "fatal"),
		strings.Contains(lower, "error"),
		strings.Contains(lower, "failed"),
		strings.Contains(lower, "timeout"),
		strings.Contains(text, "失败"),
		strings.Contains(text, "错误"),
		strings.Contains(text, "超时"):
		return "error"
	case strings.Contains(lower, "warn"),
		strings.Contains(lower, "risk"),
		strings.Contains(lower, "retry"),
		strings.Contains(text, "警告"),
		strings.Contains(text, "告警"),
		strings.Contains(text, "风险"):
		return "warn"
	default:
		return "info"
	}
}

func (s *Server) recordLiveNotification(note liveNotification) *liveNotificationEvent {
	recordedAt := time.Now().UTC()
	at := strings.TrimSpace(note.At)
	if at == "" {
		at = recordedAt.Format(time.RFC3339Nano)
	}
	event := liveNotificationEvent{
		Sequence:   atomic.AddUint64(&s.notificationSeq, 1),
		RecordedAt: recordedAt,
		At:         at,
		Level:      note.Level,
		Title:      note.Title,
		Message:    note.Message,
		Source:     note.Source,
		BrokerID:   note.BrokerID,
		Category:   note.Category,
	}

	cutoff := recordedAt.Add(-liveNotificationRetention)
	s.notificationMu.Lock()
	filtered := make([]liveNotificationEvent, 0, len(s.notificationCache)+1)
	for _, existing := range s.notificationCache {
		if existing.RecordedAt.Before(cutoff) {
			continue
		}
		filtered = append(filtered, existing)
	}
	filtered = append(filtered, event)
	if len(filtered) > maxLiveNotificationEvents {
		filtered = filtered[len(filtered)-maxLiveNotificationEvents:]
	}
	s.notificationCache = filtered
	s.notificationMu.Unlock()

	copyOfEvent := event
	return &copyOfEvent
}

func (s *Server) liveNotificationsAfter(sequence uint64) []liveNotificationEvent {
	s.notificationMu.Lock()
	defer s.notificationMu.Unlock()
	if len(s.notificationCache) == 0 {
		return nil
	}
	result := make([]liveNotificationEvent, 0, len(s.notificationCache))
	for _, event := range s.notificationCache {
		if event.Sequence <= sequence {
			continue
		}
		result = append(result, event)
	}
	return result
}

func (s *Server) writeLiveNotifications(conn *websocket.Conn, lastSequence *uint64) error {
	events := s.liveNotificationsAfter(*lastSequence)
	for _, event := range events {
		if err := conn.WriteJSON(liveNotificationEventMap(event)); err != nil {
			return err
		}
		*lastSequence = event.Sequence
	}
	return nil
}

func liveNotificationEventMap(event liveNotificationEvent) map[string]any {
	payload := map[string]any{
		"type":     "system.notification",
		"id":       fmt.Sprintf("system-notification-%d", event.Sequence),
		"at":       event.At,
		"level":    event.Level,
		"title":    event.Title,
		"source":   event.Source,
		"brokerId": event.BrokerID,
		"category": event.Category,
	}
	if event.Message != "" {
		payload["message"] = event.Message
	}
	return payload
}

func (note liveNotification) text() string {
	if note.Message == "" {
		return note.Title
	}
	return note.Title + " - " + note.Message
}

func shouldForwardNotificationToBBGO(note liveNotification) bool {
	return note.Level == "warn" || note.Level == "error" || (note.Category == "broker.connection" && note.Level == "success")
}

func liveNotificationFromFutuResponse(response *notifypb.Response) *liveNotification {
	if response == nil || response.GetRetType() != 0 || response.GetS2C() == nil {
		return nil
	}

	s2c := response.GetS2C()
	switch notifypb.NotifyType(s2c.GetType()) {
	case notifypb.NotifyType_NotifyType_GtwEvent:
		return gtwEventNotification(s2c.GetEvent())
	case notifypb.NotifyType_NotifyType_ProgramStatus:
		return programStatusNotification(s2c.GetProgramStatus())
	case notifypb.NotifyType_NotifyType_ConnStatus:
		return connStatusNotification(s2c.GetConnectStatus())
	case notifypb.NotifyType_NotifyType_QotRight:
		return qotRightNotification(s2c.GetQotRight())
	case notifypb.NotifyType_NotifyType_APIQuota:
		return apiQuotaNotification(s2c.GetApiQuota())
	default:
		note := baseFutuNotification("broker.system")
		note.Level = "info"
		note.Title = "OpenD 系统通知"
		note.Message = notifyTypeLabel(notifypb.NotifyType(s2c.GetType()))
		return &note
	}
}

func baseFutuNotification(category string) liveNotification {
	return liveNotification{
		At:       time.Now().UTC().Format(time.RFC3339Nano),
		Source:   "futu-opend",
		BrokerID: "futu",
		Category: category,
	}
}

func connStatusNotification(status *notifypb.ConnectStatus) *liveNotification {
	if status == nil {
		return nil
	}
	note := baseFutuNotification("broker.connection")
	switch {
	case status.GetQotLogined() && status.GetTrdLogined():
		note.Level = "success"
		note.Title = "OpenD 连接已恢复"
	case !status.GetQotLogined() && !status.GetTrdLogined():
		note.Level = "error"
		note.Title = "OpenD 连接状态变化"
	default:
		note.Level = "warn"
		note.Title = "OpenD 连接状态变化"
	}
	note.Message = fmt.Sprintf("行情%s，交易%s。", boolLabel(status.GetQotLogined(), "已登录", "未登录"), boolLabel(status.GetTrdLogined(), "已登录", "未登录"))
	return &note
}

func programStatusNotification(status *commonpb.ProgramStatus) *liveNotification {
	if status == nil {
		return nil
	}
	statusType := status.GetType()
	note := baseFutuNotification("broker.program")
	note.Level = programStatusLevel(statusType)
	note.Title = programStatusTitle(statusType)
	label := programStatusLabel(statusType)
	desc := strings.TrimSpace(status.GetStrExtDesc())
	if desc != "" && desc != label {
		note.Message = label + "：" + desc
	} else {
		note.Message = label
	}
	return &note
}

func gtwEventNotification(event *notifypb.GtwEvent) *liveNotification {
	if event == nil {
		return nil
	}
	eventType := notifypb.GtwEventType(event.GetType())
	note := baseFutuNotification("broker.event")
	note.Level = gtwEventLevel(eventType)
	note.Title = gtwEventTitle(eventType)
	label := gtwEventLabel(eventType)
	desc := strings.TrimSpace(event.GetDesc())
	if desc != "" && desc != label {
		note.Message = label + "：" + desc
	} else {
		note.Message = label
	}
	return &note
}

func qotRightNotification(right *notifypb.QotRight) *liveNotification {
	if right == nil {
		return nil
	}
	note := baseFutuNotification("broker.permissions")
	note.Level = "info"
	note.Title = "Futu 行情权限更新"
	summary := formatQotRightSummary(right)
	if summary == "" {
		summary = "行情权限已更新。"
	}
	note.Message = summary
	return &note
}

func apiQuotaNotification(quota *notifypb.APIQuota) *liveNotification {
	if quota == nil {
		return nil
	}
	note := baseFutuNotification("broker.quota")
	switch {
	case quota.GetRemain() <= 0:
		note.Level = "error"
	case quota.GetRemain() <= 10:
		note.Level = "warn"
	default:
		note.Level = "info"
	}
	note.Title = "Futu API 订阅额度更新"
	note.Message = fmt.Sprintf("剩余 %d，当前连接已用 %d，总已用 %d。", quota.GetRemain(), quota.GetOwnUsed(), quota.GetTotalUsed())
	return &note
}

func boolLabel(value bool, yes string, no string) string {
	if value {
		return yes
	}
	return no
}

func notifyTypeLabel(value notifypb.NotifyType) string {
	switch value {
	case notifypb.NotifyType_NotifyType_GtwEvent:
		return "OpenD 运行事件"
	case notifypb.NotifyType_NotifyType_ProgramStatus:
		return "程序状态"
	case notifypb.NotifyType_NotifyType_ConnStatus:
		return "连接状态"
	case notifypb.NotifyType_NotifyType_QotRight:
		return "行情权限"
	case notifypb.NotifyType_NotifyType_APIQuota:
		return "API 额度"
	default:
		return "系统通知"
	}
}

func programStatusLevel(value commonpb.ProgramStatusType) string {
	switch value {
	case commonpb.ProgramStatusType_ProgramStatusType_Ready:
		return "success"
	case commonpb.ProgramStatusType_ProgramStatusType_Loaded,
		commonpb.ProgramStatusType_ProgramStatusType_Loging,
		commonpb.ProgramStatusType_ProgramStatusType_NessaryDataPreparing:
		return "info"
	case commonpb.ProgramStatusType_ProgramStatusType_NeedPicVerifyCode,
		commonpb.ProgramStatusType_ProgramStatusType_NeedPhoneVerifyCode:
		return "warn"
	default:
		return "error"
	}
}

func programStatusTitle(value commonpb.ProgramStatusType) string {
	switch value {
	case commonpb.ProgramStatusType_ProgramStatusType_Ready:
		return "OpenD 已就绪"
	case commonpb.ProgramStatusType_ProgramStatusType_LoginFailed:
		return "OpenD 登录失败"
	case commonpb.ProgramStatusType_ProgramStatusType_NeedPicVerifyCode:
		return "OpenD 需要图形验证码"
	case commonpb.ProgramStatusType_ProgramStatusType_NeedPhoneVerifyCode:
		return "OpenD 需要手机验证码"
	case commonpb.ProgramStatusType_ProgramStatusType_ForceUpdate:
		return "OpenD 需要升级"
	case commonpb.ProgramStatusType_ProgramStatusType_ForceLogout:
		return "OpenD 已被强制登出"
	case commonpb.ProgramStatusType_ProgramStatusType_NessaryDataMissing:
		return "OpenD 缺少必要数据"
	case commonpb.ProgramStatusType_ProgramStatusType_UnAgreeDisclaimer:
		return "OpenD 需要确认免责声明"
	default:
		return "OpenD 程序状态更新"
	}
}

func programStatusLabel(value commonpb.ProgramStatusType) string {
	switch value {
	case commonpb.ProgramStatusType_ProgramStatusType_Loaded:
		return "已加载"
	case commonpb.ProgramStatusType_ProgramStatusType_Loging:
		return "登录中"
	case commonpb.ProgramStatusType_ProgramStatusType_NeedPicVerifyCode:
		return "需要图形验证码"
	case commonpb.ProgramStatusType_ProgramStatusType_NeedPhoneVerifyCode:
		return "需要手机验证码"
	case commonpb.ProgramStatusType_ProgramStatusType_LoginFailed:
		return "登录失败"
	case commonpb.ProgramStatusType_ProgramStatusType_ForceUpdate:
		return "需要升级客户端"
	case commonpb.ProgramStatusType_ProgramStatusType_NessaryDataPreparing:
		return "正在准备必要数据"
	case commonpb.ProgramStatusType_ProgramStatusType_NessaryDataMissing:
		return "缺少必要数据"
	case commonpb.ProgramStatusType_ProgramStatusType_UnAgreeDisclaimer:
		return "未同意免责声明"
	case commonpb.ProgramStatusType_ProgramStatusType_Ready:
		return "已就绪"
	case commonpb.ProgramStatusType_ProgramStatusType_ForceLogout:
		return "已被强制登出"
	case commonpb.ProgramStatusType_ProgramStatusType_DisclaimerPullFailed:
		return "拉取免责声明失败"
	default:
		return "程序状态已更新"
	}
}

func gtwEventLevel(value notifypb.GtwEventType) string {
	switch value {
	case notifypb.GtwEventType_GtwEventType_None:
		return "info"
	case notifypb.GtwEventType_GtwEventType_ForceUpdate,
		notifypb.GtwEventType_GtwEventType_NeedPicVerifyCode,
		notifypb.GtwEventType_GtwEventType_NeedPhoneVerifyCode,
		notifypb.GtwEventType_GtwEventType_EnableDeviceLock:
		return "warn"
	default:
		return "error"
	}
}

func gtwEventTitle(value notifypb.GtwEventType) string {
	switch value {
	case notifypb.GtwEventType_GtwEventType_LoginFailed:
		return "OpenD 登录失败"
	case notifypb.GtwEventType_GtwEventType_ForceUpdate:
		return "OpenD 需要升级"
	case notifypb.GtwEventType_GtwEventType_KickedOut:
		return "Futu 账户在别处登录"
	case notifypb.GtwEventType_GtwEventType_NeedPicVerifyCode:
		return "OpenD 需要图形验证码"
	case notifypb.GtwEventType_GtwEventType_NeedPhoneVerifyCode:
		return "OpenD 需要手机验证码"
	case notifypb.GtwEventType_GtwEventType_BanLogin:
		return "Futu 账户被禁止登录"
	default:
		return "OpenD 运行事件"
	}
}

func gtwEventLabel(value notifypb.GtwEventType) string {
	switch value {
	case notifypb.GtwEventType_GtwEventType_None:
		return "无异常"
	case notifypb.GtwEventType_GtwEventType_LocalCfgLoadFailed:
		return "加载本地配置失败"
	case notifypb.GtwEventType_GtwEventType_APISvrRunFailed:
		return "OpenD 服务启动失败"
	case notifypb.GtwEventType_GtwEventType_ForceUpdate:
		return "客户端版本过低"
	case notifypb.GtwEventType_GtwEventType_LoginFailed:
		return "登录失败"
	case notifypb.GtwEventType_GtwEventType_UnAgreeDisclaimer:
		return "未同意免责声明"
	case notifypb.GtwEventType_GtwEventType_NetCfgMissing:
		return "缺少必要网络配置"
	case notifypb.GtwEventType_GtwEventType_KickedOut:
		return "账户在别处登录"
	case notifypb.GtwEventType_GtwEventType_LoginPwdChanged:
		return "登录密码已修改"
	case notifypb.GtwEventType_GtwEventType_BanLogin:
		return "用户被禁止登录"
	case notifypb.GtwEventType_GtwEventType_NeedPicVerifyCode:
		return "需要图形验证码"
	case notifypb.GtwEventType_GtwEventType_NeedPhoneVerifyCode:
		return "需要手机验证码"
	case notifypb.GtwEventType_GtwEventType_AppDataNotExist:
		return "程序自带数据不存在"
	case notifypb.GtwEventType_GtwEventType_NessaryDataMissing:
		return "缺少必要数据"
	case notifypb.GtwEventType_GtwEventType_TradePwdChanged:
		return "交易密码已修改"
	case notifypb.GtwEventType_GtwEventType_EnableDeviceLock:
		return "已启用设备锁"
	default:
		return "运行事件已更新"
	}
}

func formatQotRightSummary(right *notifypb.QotRight) string {
	parts := make([]string, 0, 8)
	appendRight := func(label string, value *int32) {
		if value == nil {
			return
		}
		parts = append(parts, fmt.Sprintf("%s %s", label, qotRightLabel(*value)))
	}
	appendRight("HK", right.HkQotRight)
	appendRight("HK Option", right.HkOptionQotRight)
	appendRight("HK Future", right.HkFutureQotRight)
	appendRight("US", right.UsQotRight)
	appendRight("US Option", right.UsOptionQotRight)
	appendRight("CN", right.CnQotRight)
	appendRight("US Index", right.UsIndexQotRight)
	appendRight("US OTC", right.UsOTCQotRight)
	appendRight("SG Future", right.SgFutureQotRight)
	appendRight("JP Future", right.JpFutureQotRight)
	appendRight("CME", right.UsFutureQotRightCME)
	appendRight("CBOT", right.UsFutureQotRightCBOT)
	appendRight("NYMEX", right.UsFutureQotRightNYMEX)
	appendRight("COMEX", right.UsFutureQotRightCOMEX)
	appendRight("CBOE", right.UsFutureQotRightCBOE)
	return strings.Join(parts, "；")
}

func qotRightLabel(value int32) string {
	switch qotcommonpb.QotRight(value) {
	case qotcommonpb.QotRight_QotRight_Bmp:
		return "BMP"
	case qotcommonpb.QotRight_QotRight_Level1:
		return "Level 1"
	case qotcommonpb.QotRight_QotRight_Level2:
		return "Level 2"
	case qotcommonpb.QotRight_QotRight_Level3:
		return "Level 3"
	case qotcommonpb.QotRight_QotRight_SF:
		return "高级行情"
	case qotcommonpb.QotRight_QotRight_No:
		return "无权限"
	default:
		return "未知"
	}
}
