package servercore

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestLiveNotificationFromBBGONotifyFormatsStringArgs(t *testing.T) {
	note := liveNotificationFromBBGONotify("strategy %s retry warning", "demo-grid")
	if note == nil {
		t.Fatal("expected note")
	}
	if note.Title != "BBGO 通知" {
		t.Fatalf("title = %q", note.Title)
	}
	if note.Message != "strategy demo-grid retry warning" {
		t.Fatalf("message = %q", note.Message)
	}
	if note.Level != "warn" {
		t.Fatalf("level = %q", note.Level)
	}
	if note.Source != "bbgo.notify" {
		t.Fatalf("source = %q", note.Source)
	}
	if note.Category != "bbgo.notify" {
		t.Fatalf("category = %q", note.Category)
	}
	if strings.TrimSpace(note.At) == "" {
		t.Fatal("expected timestamp")
	}
}

func TestLiveNotificationFromBBGONotifyHandlesErrorsStringersAndForwardedNotes(t *testing.T) {
	if note := liveNotificationFromBBGONotify(nil); note != nil {
		t.Fatalf("nil bbgo notify = %#v, want nil", note)
	}
	forwarded := liveNotification{Title: "OpenD 连接状态变化", Message: "行情未登录，交易已登录。"}
	if note := liveNotificationFromBBGONotify(forwardedBBGONotification{note: forwarded}); note != nil {
		t.Fatalf("forwarded notification = %#v, want nil to avoid loops", note)
	}

	errNote := liveNotificationFromBBGONotify(assertStringer("risk engine timeout"), "retry")
	if errNote == nil || errNote.Level != "error" || errNote.Message != "risk engine timeout retry" {
		t.Fatalf("stringer note = %+v, want error-level timeout message", errNote)
	}
	errorObjNote := liveNotificationFromBBGONotify(assertError("order submit failed"))
	if errorObjNote == nil || errorObjNote.Title != "BBGO 错误" || errorObjNote.Level != "error" || errorObjNote.Message != "order submit failed" {
		t.Fatalf("error note = %+v", errorObjNote)
	}

	if text := liveNotificationText(liveNotification{Title: "标题"}); text != "标题" {
		t.Fatalf("title-only text = %q", text)
	}
	if text := liveNotificationText(liveNotification{Title: "标题", Message: "内容"}); text != "标题 - 内容" {
		t.Fatalf("title/message text = %q", text)
	}
	if !shouldForwardNotificationToBBGO(liveNotification{Level: "warn"}) ||
		!shouldForwardNotificationToBBGO(liveNotification{Level: "error"}) ||
		!shouldForwardNotificationToBBGO(liveNotification{Level: "success", Category: "broker.connection"}) ||
		shouldForwardNotificationToBBGO(liveNotification{Level: "info", Category: "broker.quota"}) {
		t.Fatal("shouldForwardNotificationToBBGO did not match broker alert policy")
	}
}

func TestBBGONotificationBridgeStartStopStringAndUpload(t *testing.T) {
	forwarded := forwardedBBGONotification{note: liveNotification{Title: "OpenD", Message: "connected"}}
	if got := forwarded.String(); got != "OpenD - connected" {
		t.Fatalf("forwarded String() = %q", got)
	}

	source := bbgoNotificationSource{}
	stop, err := source.Start(nil)
	if err != nil || stop != nil {
		t.Fatalf("Start nil sink = %v/%v, want nil nil", stop, err)
	}

	notifications := make(chan liveNotification, 4)
	stop, err = source.Start(func(note liveNotification) *liveNotificationEvent {
		notifications <- note
		return &liveNotificationEvent{Sequence: 1, At: note.At, Level: note.Level, Title: note.Title, Message: note.Message, Source: note.Source, Category: note.Category}
	})
	if err != nil || stop == nil {
		t.Fatalf("Start sink = %v/%v", stop, err)
	}
	dispatchBBGONotification(liveNotification{Title: "bridge", Message: "online"})
	select {
	case note := <-notifications:
		if note.Title != "bridge" || note.Message != "online" {
			t.Fatalf("dispatched note = %+v", note)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bridge notification")
	}

	liveSocketBBGONotifier{}.Upload(nil)
	liveSocketBBGONotifier{}.Upload(&bbgotypes.UploadFile{Caption: "report ready", FileType: bbgotypes.FileTypeDocument})
	select {
	case note := <-notifications:
		if note.Category != "bbgo.upload" || note.Message != "report ready" {
			t.Fatalf("upload note = %+v", note)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for upload notification")
	}
	liveSocketBBGONotifier{}.Upload(&bbgotypes.UploadFile{FileType: bbgotypes.FileTypeText})
	select {
	case note := <-notifications:
		if note.Category != "bbgo.upload" || note.Message != "text" {
			t.Fatalf("upload fallback note = %+v", note)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for upload fallback notification")
	}

	if err := stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := stop(); err != nil {
		t.Fatalf("second stop: %v", err)
	}
	dispatchBBGONotification(liveNotification{Title: "after stop"})
	select {
	case note := <-notifications:
		t.Fatalf("received notification after stop: %+v", note)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestExchangeCalendarAlertRecordingHonorsNotificationSetting(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	alert := exchangecalendar.SourceAlert{
		SourceID: "nyse_official",
		Market:   "US",
		Level:    "warn",
		Kind:     "fetch_failed",
		Title:    "交易所日历源抓取失败",
		Message:  "US 市场日历源 nyse_official 抓取失败。",
	}

	server.recordExchangeCalendarAlert(alert)
	if got := len(server.liveNotificationsAfter(0)); got != 1 {
		t.Fatalf("notifications with default setting = %d, want 1", got)
	}

	var disabled jfsettings.ExchangeCalendarSettings
	if err := json.Unmarshal([]byte(`{"autoRefreshEnabled":true,"errorNotificationsEnabled":false,"refreshIntervalHours":24,"warmupMarkets":["US"]}`), &disabled); err != nil {
		t.Fatalf("Unmarshal settings: %v", err)
	}
	if _, err := store.SaveExchangeCalendarSettings(disabled); err != nil {
		t.Fatalf("SaveExchangeCalendarSettings: %v", err)
	}
	server.recordExchangeCalendarAlert(alert)
	if got := len(server.liveNotificationsAfter(0)); got != 1 {
		t.Fatalf("notifications after disabling = %d, want unchanged 1", got)
	}
}

func TestLiveNotificationFromFutuResponseMapsConnectionProgramEventAndRights(t *testing.T) {
	if note := liveNotificationFromFutuResponse(nil); note != nil {
		t.Fatalf("nil futu response = %#v, want nil", note)
	}
	if note := liveNotificationFromFutuResponse(&notifypb.Response{RetType: servercoreInt32(1)}); note != nil {
		t.Fatalf("nonzero ret futu response = %#v, want nil", note)
	}

	connectionNote := liveNotificationFromFutuResponse(&notifypb.Response{
		RetType: servercoreInt32(0),
		S2C: &notifypb.S2C{
			Type: servercoreInt32(int32(notifypb.NotifyType_NotifyType_ConnStatus)),
			ConnectStatus: &notifypb.ConnectStatus{
				QotLogined: new(false),
				TrdLogined: new(true),
			},
		},
	})
	if connectionNote == nil || connectionNote.Level != "warn" || connectionNote.Category != "broker.connection" || !strings.Contains(connectionNote.Message, "行情未登录") {
		t.Fatalf("connection note = %+v", connectionNote)
	}

	programType := commonpb.ProgramStatusType_ProgramStatusType_NeedPhoneVerifyCode
	desc := "手机号尾号 1234"
	programNote := liveNotificationFromFutuResponse(&notifypb.Response{
		RetType: servercoreInt32(0),
		S2C: &notifypb.S2C{
			Type: servercoreInt32(int32(notifypb.NotifyType_NotifyType_ProgramStatus)),
			ProgramStatus: &notifypb.ProgramStatus{
				ProgramStatus: &commonpb.ProgramStatus{
					Type:       &programType,
					StrExtDesc: &desc,
				},
			},
		},
	})
	if programNote == nil || programNote.Level != "warn" || programNote.Title != "OpenD 需要手机验证码" || !strings.Contains(programNote.Message, desc) {
		t.Fatalf("program note = %+v", programNote)
	}

	eventDesc := "交易账号在另一终端登录"
	eventNote := liveNotificationFromFutuResponse(&notifypb.Response{
		RetType: servercoreInt32(0),
		S2C: &notifypb.S2C{
			Type: servercoreInt32(int32(notifypb.NotifyType_NotifyType_GtwEvent)),
			Event: &notifypb.GtwEvent{
				EventType: servercoreInt32(int32(notifypb.GtwEventType_GtwEventType_KickedOut)),
				Desc:      &eventDesc,
			},
		},
	})
	if eventNote == nil || eventNote.Level != "error" || eventNote.Title != "Futu 账户在别处登录" || !strings.Contains(eventNote.Message, eventDesc) {
		t.Fatalf("event note = %+v", eventNote)
	}

	hkRight := int32(qotcommonpb.QotRight_QotRight_No)
	usRight := int32(qotcommonpb.QotRight_QotRight_Level2)
	cnRight := int32(qotcommonpb.QotRight_QotRight_No)
	rightNote := liveNotificationFromFutuResponse(&notifypb.Response{
		RetType: servercoreInt32(0),
		S2C: &notifypb.S2C{
			Type: servercoreInt32(int32(notifypb.NotifyType_NotifyType_QotRight)),
			QotRight: &notifypb.QotRight{
				HkQotRight: &hkRight,
				UsQotRight: &usRight,
				CnQotRight: &cnRight,
			},
		},
	})
	if rightNote == nil || rightNote.Category != "broker.permissions" || !strings.Contains(rightNote.Message, "HK 无权限") || !strings.Contains(rightNote.Message, "US Level 2") {
		t.Fatalf("right note = %+v", rightNote)
	}

	unknownNote := liveNotificationFromFutuResponse(&notifypb.Response{
		RetType: servercoreInt32(0),
		S2C:     &notifypb.S2C{Type: servercoreInt32(9999)},
	})
	if unknownNote == nil || unknownNote.Level != "info" || unknownNote.Message != "系统通知" {
		t.Fatalf("unknown note = %+v", unknownNote)
	}
}

func TestLiveNotificationFromFutuResponseMapsAPIQuota(t *testing.T) {
	note := liveNotificationFromFutuResponse(&notifypb.Response{
		RetType: new(int32(0)),
		S2C: &notifypb.S2C{
			Type: new(int32(notifypb.NotifyType_NotifyType_APIQuota)),
			ApiQuota: &notifypb.APIQuota{
				SubQuota:       new(int32(1000)),
				HistoryKLQuota: new(int32(300)),
			},
		},
	})
	if note == nil {
		t.Fatal("expected note")
	}
	if note.Title != "Futu API 额度更新" {
		t.Fatalf("title = %q", note.Title)
	}
	if note.Level != "info" {
		t.Fatalf("level = %q", note.Level)
	}
	if note.Source != "futu-opend" {
		t.Fatalf("source = %q", note.Source)
	}
	if note.BrokerID != "futu" {
		t.Fatalf("brokerId = %q", note.BrokerID)
	}
	if note.Category != "broker.quota" {
		t.Fatalf("category = %q", note.Category)
	}
	if note.Message != "订阅额度 1000，历史 K 线额度 300。" {
		t.Fatalf("message = %q", note.Message)
	}
	if strings.TrimSpace(note.At) == "" {
		t.Fatal("expected timestamp")
	}
}

func TestFutuNotificationStatusLabelMatrices(t *testing.T) {
	if connStatusNotification(nil) != nil || programStatusNotification(nil) != nil || gtwEventNotification(nil) != nil ||
		qotRightNotification(nil) != nil || apiQuotaNotification(nil) != nil || usedQuotaNotification(nil) != nil {
		t.Fatal("nil Futu notification payloads should be ignored")
	}

	if note := connStatusNotification(&notifypb.ConnectStatus{QotLogined: new(true), TrdLogined: new(true)}); note == nil || note.Level != "success" {
		t.Fatalf("connected status note = %+v", note)
	}
	if note := connStatusNotification(&notifypb.ConnectStatus{QotLogined: new(false), TrdLogined: new(false)}); note == nil || note.Level != "error" {
		t.Fatalf("disconnected status note = %+v", note)
	}

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
			if got := programStatusLevel(tt.status); got != tt.level {
				t.Fatalf("programStatusLevel = %q, want %q", got, tt.level)
			}
			if got := programStatusTitle(tt.status); got != tt.title {
				t.Fatalf("programStatusTitle = %q, want %q", got, tt.title)
			}
			if got := programStatusLabel(tt.status); got != tt.label {
				t.Fatalf("programStatusLabel = %q, want %q", got, tt.label)
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
		t.Run("event_"+tt.label, func(t *testing.T) {
			if got := gtwEventLevel(tt.event); got != tt.level {
				t.Fatalf("gtwEventLevel = %q, want %q", got, tt.level)
			}
			if got := gtwEventTitle(tt.event); got != tt.title {
				t.Fatalf("gtwEventTitle = %q, want %q", got, tt.title)
			}
			if got := gtwEventLabel(tt.event); got != tt.label {
				t.Fatalf("gtwEventLabel = %q, want %q", got, tt.label)
			}
		})
	}

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
		if got := notifyTypeLabel(tt.value); got != tt.label {
			t.Fatalf("notifyTypeLabel(%v) = %q, want %q", tt.value, got, tt.label)
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
		if got := qotRightLabel(int32(tt.value)); got != tt.label {
			t.Fatalf("qotRightLabel(%v) = %q, want %q", tt.value, got, tt.label)
		}
	}
	if note := qotRightNotification(&notifypb.QotRight{}); note == nil || note.Message != "行情权限已更新。" {
		t.Fatalf("empty qot right note = %+v", note)
	}

	if note := apiQuotaNotification(&notifypb.APIQuota{}); note == nil || note.Level != "info" {
		t.Fatalf("quota note=%+v, want info", note)
	}
	if note := usedQuotaNotification(&notifypb.UsedQuota{}); note == nil || note.Level != "info" {
		t.Fatalf("used quota note=%+v, want info", note)
	}
}

func TestLiveNotificationFromExchangeCalendarAlertMapsSourceAndCategory(t *testing.T) {
	note := liveNotificationFromExchangeCalendarAlert(exchangecalendar.SourceAlert{
		SourceID: "nyse_official",
		Market:   "US",
		Level:    "error",
		Kind:     "structure_changed",
		Title:    "交易所日历源解析异常",
		Message:  "US 市场日历源 nyse_official 抓取成功但未解析到有效交易日。",
	})
	if note == nil {
		t.Fatal("expected note")
	}
	if note.Level != "error" {
		t.Fatalf("level = %q", note.Level)
	}
	if note.Source != "exchange-calendars" {
		t.Fatalf("source = %q", note.Source)
	}
	if note.Category != "market.calendar.source" {
		t.Fatalf("category = %q", note.Category)
	}
	if strings.TrimSpace(note.At) == "" {
		t.Fatal("expected timestamp")
	}
}

type assertStringer string

func (s assertStringer) String() string { return string(s) }

type assertError string

func (e assertError) Error() string { return string(e) }

func servercoreInt32(value int32) *int32 { return new(value) }
