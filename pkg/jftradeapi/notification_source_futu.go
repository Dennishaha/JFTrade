package jftradeapi

import (
	"fmt"
	"strings"
	"time"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

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
