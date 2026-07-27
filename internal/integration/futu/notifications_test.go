package futu

import (
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/live"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestLiveNotificationFromResponseRoutesProtocolPayloadsToNeutralCategories(t *testing.T) {
	if note := LiveNotificationFromResponse(nil); note != nil {
		t.Fatalf("nil response = %#v", note)
	}
	if note := LiveNotificationFromResponse(&notifypb.Response{RetType: new(int32(1))}); note != nil {
		t.Fatalf("failed response = %#v", note)
	}
	if note := LiveNotificationFromResponse(&notifypb.Response{RetType: new(int32(0))}); note != nil {
		t.Fatalf("missing payload response = %#v", note)
	}

	connection := LiveNotificationFromResponse(notificationResponse(
		notifypb.NotifyType_NotifyType_ConnStatus,
		&notifypb.S2C{ConnectStatus: &notifypb.ConnectStatus{QotLogined: new(false), TrdLogined: new(true)}},
	))
	assertNeutralNotification(t, connection, "warn", "broker.connection")
	if !strings.Contains(connection.Message, "行情未登录") || !strings.Contains(connection.Message, "交易已登录") {
		t.Fatalf("connection message = %q", connection.Message)
	}

	programType := commonpb.ProgramStatusType_ProgramStatusType_NeedPhoneVerifyCode
	description := "手机号尾号 1234"
	program := LiveNotificationFromResponse(notificationResponse(
		notifypb.NotifyType_NotifyType_ProgramStatus,
		&notifypb.S2C{ProgramStatus: &notifypb.ProgramStatus{ProgramStatus: &commonpb.ProgramStatus{
			Type: &programType, StrExtDesc: &description,
		}}},
	))
	assertNeutralNotification(t, program, "warn", "broker.program")
	if program.Title != "OpenD 需要手机验证码" || !strings.Contains(program.Message, description) {
		t.Fatalf("program notification = %#v", program)
	}

	eventDescription := "交易账号在另一终端登录"
	event := LiveNotificationFromResponse(notificationResponse(
		notifypb.NotifyType_NotifyType_GtwEvent,
		&notifypb.S2C{Event: &notifypb.GtwEvent{
			EventType: new(int32(notifypb.GtwEventType_GtwEventType_KickedOut)),
			Desc:      &eventDescription,
		}},
	))
	assertNeutralNotification(t, event, "error", "broker.event")
	if event.Title != "Futu 账户在别处登录" || !strings.Contains(event.Message, eventDescription) {
		t.Fatalf("gateway event notification = %#v", event)
	}

	hkRight := int32(qotcommonpb.QotRight_QotRight_No)
	usRight := int32(qotcommonpb.QotRight_QotRight_Level2)
	rights := LiveNotificationFromResponse(notificationResponse(
		notifypb.NotifyType_NotifyType_QotRight,
		&notifypb.S2C{QotRight: &notifypb.QotRight{HkQotRight: &hkRight, UsQotRight: &usRight}},
	))
	assertNeutralNotification(t, rights, "info", "broker.permissions")
	if !strings.Contains(rights.Message, "HK 无权限") || !strings.Contains(rights.Message, "US Level 2") {
		t.Fatalf("quote-right notification = %#v", rights)
	}

	apiQuota := LiveNotificationFromResponse(notificationResponse(
		notifypb.NotifyType_NotifyType_APIQuota,
		&notifypb.S2C{ApiQuota: &notifypb.APIQuota{SubQuota: new(int32(1000)), HistoryKLQuota: new(int32(300))}},
	))
	assertNeutralNotification(t, apiQuota, "info", "broker.quota")
	if apiQuota.Message != "订阅额度 1000，历史 K 线额度 300。" {
		t.Fatalf("API quota message = %q", apiQuota.Message)
	}

	usedQuota := LiveNotificationFromResponse(notificationResponse(
		notifypb.NotifyType_NotifyType_UsedQuota,
		&notifypb.S2C{UsedQuota: &notifypb.UsedQuota{UsedSubQuota: new(int32(9)), UsedKLineQuota: new(int32(7))}},
	))
	assertNeutralNotification(t, usedQuota, "info", "broker.quota")
	if usedQuota.Message != "已使用订阅额度 9，已使用历史 K 线额度 7。" {
		t.Fatalf("used quota message = %q", usedQuota.Message)
	}

	unknown := LiveNotificationFromResponse(notificationResponse(
		notifypb.NotifyType(9999),
		&notifypb.S2C{},
	))
	assertNeutralNotification(t, unknown, "info", "broker.system")
	if unknown.Message != "系统通知" {
		t.Fatalf("unknown notification = %#v", unknown)
	}
}

func TestNeutralNotificationBuildersHandleNilAndStatusTransitions(t *testing.T) {
	if ConnectionStatusNotification(nil) != nil ||
		ProgramStatusNotification(nil) != nil ||
		GatewayEventNotification(nil) != nil ||
		QuoteRightNotification(nil) != nil ||
		APIQuotaNotification(nil) != nil ||
		UsedQuotaNotification(nil) != nil {
		t.Fatal("nil protocol payload should not create a notification")
	}

	connected := ConnectionStatusNotification(&notifypb.ConnectStatus{QotLogined: new(true), TrdLogined: new(true)})
	assertNeutralNotification(t, connected, "success", "broker.connection")
	disconnected := ConnectionStatusNotification(&notifypb.ConnectStatus{QotLogined: new(false), TrdLogined: new(false)})
	assertNeutralNotification(t, disconnected, "error", "broker.connection")

	ready := commonpb.ProgramStatusType_ProgramStatusType_Ready
	readyLabel := "已就绪"
	if note := ProgramStatusNotification(&commonpb.ProgramStatus{Type: &ready, StrExtDesc: &readyLabel}); note == nil || note.Message != readyLabel {
		t.Fatalf("program description equal to label = %#v", note)
	}
	noneEvent := int32(notifypb.GtwEventType_GtwEventType_None)
	noneLabel := "无异常"
	if note := GatewayEventNotification(&notifypb.GtwEvent{EventType: &noneEvent, Desc: &noneLabel}); note == nil || note.Message != noneLabel {
		t.Fatalf("gateway description equal to label = %#v", note)
	}

	if note := QuoteRightNotification(&notifypb.QotRight{}); note == nil || note.Message != "行情权限已更新。" {
		t.Fatalf("empty quote rights = %#v", note)
	}
	if note := APIQuotaNotification(&notifypb.APIQuota{}); note == nil || note.Message != "订阅额度 0，历史 K 线额度 0。" {
		t.Fatalf("empty API quota = %#v", note)
	}
	if note := UsedQuotaNotification(&notifypb.UsedQuota{}); note == nil || note.Message != "已使用订阅额度 0，已使用历史 K 线额度 0。" {
		t.Fatalf("empty used quota = %#v", note)
	}
}

func TestNotificationLabelsCoverEverySupportedProgramAndGatewayState(t *testing.T) {
	for _, tt := range []struct {
		status commonpb.ProgramStatusType
		level  string
		title  string
		label  string
	}{
		{commonpb.ProgramStatusType_ProgramStatusType_Loaded, "info", "OpenD 程序状态更新", "已加载"},
		{commonpb.ProgramStatusType_ProgramStatusType_Loging, "info", "OpenD 程序状态更新", "登录中"},
		{commonpb.ProgramStatusType_ProgramStatusType_NeedPicVerifyCode, "warn", "OpenD 需要图形验证码", "需要图形验证码"},
		{commonpb.ProgramStatusType_ProgramStatusType_NeedPhoneVerifyCode, "warn", "OpenD 需要手机验证码", "需要手机验证码"},
		{commonpb.ProgramStatusType_ProgramStatusType_LoginFailed, "error", "OpenD 登录失败", "登录失败"},
		{commonpb.ProgramStatusType_ProgramStatusType_ForceUpdate, "error", "OpenD 需要升级", "需要升级客户端"},
		{commonpb.ProgramStatusType_ProgramStatusType_NessaryDataPreparing, "info", "OpenD 程序状态更新", "正在准备必要数据"},
		{commonpb.ProgramStatusType_ProgramStatusType_NessaryDataMissing, "error", "OpenD 缺少必要数据", "缺少必要数据"},
		{commonpb.ProgramStatusType_ProgramStatusType_UnAgreeDisclaimer, "error", "OpenD 需要确认免责声明", "未同意免责声明"},
		{commonpb.ProgramStatusType_ProgramStatusType_Ready, "success", "OpenD 已就绪", "已就绪"},
		{commonpb.ProgramStatusType_ProgramStatusType_ForceLogout, "error", "OpenD 已被强制登出", "已被强制登出"},
		{commonpb.ProgramStatusType_ProgramStatusType_DisclaimerPullFailed, "error", "OpenD 程序状态更新", "拉取免责声明失败"},
		{commonpb.ProgramStatusType(9999), "error", "OpenD 程序状态更新", "程序状态已更新"},
	} {
		t.Run("program_"+tt.label, func(t *testing.T) {
			if got := ProgramStatusLevel(tt.status); got != tt.level {
				t.Fatalf("level = %q, want %q", got, tt.level)
			}
			if got := ProgramStatusTitle(tt.status); got != tt.title {
				t.Fatalf("title = %q, want %q", got, tt.title)
			}
			if got := ProgramStatusLabel(tt.status); got != tt.label {
				t.Fatalf("label = %q, want %q", got, tt.label)
			}
		})
	}

	for _, tt := range []struct {
		event notifypb.GtwEventType
		level string
		title string
		label string
	}{
		{notifypb.GtwEventType_GtwEventType_None, "info", "OpenD 运行事件", "无异常"},
		{notifypb.GtwEventType_GtwEventType_LocalCfgLoadFailed, "error", "OpenD 运行事件", "加载本地配置失败"},
		{notifypb.GtwEventType_GtwEventType_APISvrRunFailed, "error", "OpenD 运行事件", "OpenD 服务启动失败"},
		{notifypb.GtwEventType_GtwEventType_ForceUpdate, "warn", "OpenD 需要升级", "客户端版本过低"},
		{notifypb.GtwEventType_GtwEventType_LoginFailed, "error", "OpenD 登录失败", "登录失败"},
		{notifypb.GtwEventType_GtwEventType_UnAgreeDisclaimer, "error", "OpenD 运行事件", "未同意免责声明"},
		{notifypb.GtwEventType_GtwEventType_NetCfgMissing, "error", "OpenD 运行事件", "缺少必要网络配置"},
		{notifypb.GtwEventType_GtwEventType_KickedOut, "error", "Futu 账户在别处登录", "账户在别处登录"},
		{notifypb.GtwEventType_GtwEventType_LoginPwdChanged, "error", "OpenD 运行事件", "登录密码已修改"},
		{notifypb.GtwEventType_GtwEventType_BanLogin, "error", "Futu 账户被禁止登录", "用户被禁止登录"},
		{notifypb.GtwEventType_GtwEventType_NeedPicVerifyCode, "warn", "OpenD 需要图形验证码", "需要图形验证码"},
		{notifypb.GtwEventType_GtwEventType_NeedPhoneVerifyCode, "warn", "OpenD 需要手机验证码", "需要手机验证码"},
		{notifypb.GtwEventType_GtwEventType_AppDataNotExist, "error", "OpenD 运行事件", "程序自带数据不存在"},
		{notifypb.GtwEventType_GtwEventType_NessaryDataMissing, "error", "OpenD 运行事件", "缺少必要数据"},
		{notifypb.GtwEventType_GtwEventType_TradePwdChanged, "error", "OpenD 运行事件", "交易密码已修改"},
		{notifypb.GtwEventType_GtwEventType_EnableDeviceLock, "warn", "OpenD 运行事件", "已启用设备锁"},
		{notifypb.GtwEventType(9999), "error", "OpenD 运行事件", "运行事件已更新"},
	} {
		t.Run("gateway_"+tt.label, func(t *testing.T) {
			if got := GatewayEventLevel(tt.event); got != tt.level {
				t.Fatalf("level = %q, want %q", got, tt.level)
			}
			if got := GatewayEventTitle(tt.event); got != tt.title {
				t.Fatalf("title = %q, want %q", got, tt.title)
			}
			if got := GatewayEventLabel(tt.event); got != tt.label {
				t.Fatalf("label = %q, want %q", got, tt.label)
			}
		})
	}
}

func TestNotificationAndQuoteRightLabelsRemainStable(t *testing.T) {
	for _, tt := range []struct {
		value notifypb.NotifyType
		label string
	}{
		{notifypb.NotifyType_NotifyType_GtwEvent, "OpenD 运行事件"},
		{notifypb.NotifyType_NotifyType_ProgramStatus, "程序状态"},
		{notifypb.NotifyType_NotifyType_ConnStatus, "连接状态"},
		{notifypb.NotifyType_NotifyType_QotRight, "行情权限"},
		{notifypb.NotifyType_NotifyType_APIQuota, "API 额度"},
		{notifypb.NotifyType_NotifyType_UsedQuota, "已使用额度"},
		{notifypb.NotifyType(9999), "系统通知"},
	} {
		if got := NotifyTypeLabel(tt.value); got != tt.label {
			t.Fatalf("NotifyTypeLabel(%v) = %q, want %q", tt.value, got, tt.label)
		}
	}

	for _, tt := range []struct {
		value qotcommonpb.QotRight
		label string
	}{
		{qotcommonpb.QotRight_QotRight_Bmp, "BMP"},
		{qotcommonpb.QotRight_QotRight_Level1, "Level 1"},
		{qotcommonpb.QotRight_QotRight_Level2, "Level 2"},
		{qotcommonpb.QotRight_QotRight_Level3, "Level 3"},
		{qotcommonpb.QotRight_QotRight_SF, "高级行情"},
		{qotcommonpb.QotRight_QotRight_No, "无权限"},
		{qotcommonpb.QotRight_QotRight_Unknow, "未知"},
	} {
		if got := QuoteRightLabel(int32(tt.value)); got != tt.label {
			t.Fatalf("QuoteRightLabel(%v) = %q, want %q", tt.value, got, tt.label)
		}
	}
}

func notificationResponse(kind notifypb.NotifyType, payload *notifypb.S2C) *notifypb.Response {
	payload.Type = new(int32(kind))
	return &notifypb.Response{RetType: new(int32(0)), S2C: payload}
}

func assertNeutralNotification(t *testing.T, note *live.Notification, level, category string) {
	t.Helper()
	if note == nil {
		t.Fatal("notification = nil")
	}
	if note.Level != level || note.Category != category {
		t.Fatalf("notification level/category = %q/%q, want %q/%q: %#v", note.Level, note.Category, level, category, note)
	}
	if note.Source != "futu-opend" || note.BrokerID != "futu" || strings.TrimSpace(note.At) == "" {
		t.Fatalf("notification identity = %#v", note)
	}
}
