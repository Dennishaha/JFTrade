package futu

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	trading "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	globalpb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
)

// MinimumOpenDVersion is the minimum OpenD version accepted by the runtime.
const MinimumOpenDVersion = opend.MinimumOpenDVersion

// ProbeConfig is the narrow connection input required for an OpenD health probe.
type ProbeConfig struct {
	Host         string
	APIPort      int
	WebSocketKey string
}

// Probe is a broker-neutral snapshot of OpenD connectivity and session state.
type Probe struct {
	CheckedAt        string
	Connectivity     string
	Status           string
	IssueCode        string
	LastError        *string
	QuoteLoggedIn    *bool
	TradeLoggedIn    *bool
	ServerVersion    *string
	ProgramStatus    *string
	ProgramTimestamp *string
	Markets          []trading.BrokerRuntimeMarketState
}

// ProbeOpenD performs a bounded OpenD handshake, validates the supported
// version and returns only protocol-neutral data to callers.
func ProbeOpenD(ctx context.Context, config ProbeConfig) Probe {
	checkedAt := time.Now().UTC().Format(time.RFC3339Nano)
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	client := opend.New(opend.Config{
		Addr:             net.JoinHostPort(config.Host, strconv.Itoa(config.APIPort)),
		WebSocketKey:     config.WebSocketKey,
		HandshakeTimeout: 2 * time.Second,
		RequestTimeout:   3 * time.Second,
	})
	defer func() { besteffort.LogError(client.Close()) }()
	if err := client.Connect(probeCtx); err != nil {
		return Probe{CheckedAt: checkedAt, Connectivity: "disconnected", Status: "offline", LastError: new(err.Error())}
	}

	initReq := &initpb.Request{C2S: &initpb.C2S{
		ClientVer: new(int32(101)), ClientID: new("jftrade-api"),
		RecvNotify: new(false), ProgrammingLanguage: new("Go"),
	}}
	var initResp initpb.Response
	if err := client.Call(probeCtx, opend.ProtoInitConnect, initReq, &initResp); err != nil {
		return degradedProbe(checkedAt, err.Error())
	}
	if initResp.GetRetType() != int32(commonpb.RetType_RetType_Succeed) {
		message := initResp.GetRetMsg()
		if message == "" {
			message = fmt.Sprintf("InitConnect failed: retType=%d", initResp.GetRetType())
		}
		return degradedProbe(checkedAt, message)
	}

	globalReq := &globalpb.Request{C2S: &globalpb.C2S{UserID: new(uint64(0))}}
	var globalResp globalpb.Response
	if err := client.Call(probeCtx, opend.ProtoGetGlobalState, globalReq, &globalResp); err != nil {
		return degradedProbe(checkedAt, err.Error())
	}
	if globalResp.GetRetType() != int32(commonpb.RetType_RetType_Succeed) {
		message := globalResp.GetRetMsg()
		if message == "" {
			message = fmt.Sprintf("GetGlobalState failed: retType=%d", globalResp.GetRetType())
		}
		return degradedProbe(checkedAt, message)
	}
	return ProbeFromGlobalState(checkedAt, globalResp.GetS2C())
}

func degradedProbe(checkedAt, message string) Probe {
	return Probe{CheckedAt: checkedAt, Connectivity: "degraded", Status: "degraded", LastError: &message}
}

// ProbeFromGlobalState converts OpenD's protobuf state at the integration
// boundary. It is exported to support protocol fixture tests in this package.
func ProbeFromGlobalState(checkedAt string, state *globalpb.S2C) Probe {
	if state == nil {
		return degradedProbe(checkedAt, "GetGlobalState returned no server state")
	}
	serverVersion := opend.FormatVersion(state.GetServerVer(), state.GetServerBuildNo())
	serverBuildNo := state.GetServerBuildNo()
	if err := opend.ValidateMinimumVersion(state.GetServerVer(), &serverBuildNo); err != nil {
		message := err.Error()
		return Probe{
			CheckedAt: checkedAt, Connectivity: "degraded", Status: "degraded",
			IssueCode: "OPEND_VERSION_UNSUPPORTED", LastError: &message, ServerVersion: &serverVersion,
		}
	}
	return Probe{
		CheckedAt: checkedAt, Connectivity: "connected", Status: "healthy",
		QuoteLoggedIn: new(state.GetQotLogined()), TradeLoggedIn: new(state.GetTrdLogined()),
		ServerVersion: &serverVersion, ProgramStatus: new(ProgramStatusString(state.GetProgramStatus())),
		ProgramTimestamp: new(time.Unix(state.GetTime(), 0).UTC().Format(time.RFC3339Nano)),
		Markets: []trading.BrokerRuntimeMarketState{
			{Market: "HK", State: strconv.Itoa(int(state.GetMarketHK()))},
			{Market: "US", State: strconv.Itoa(int(state.GetMarketUS()))},
			{Market: "SH", State: strconv.Itoa(int(state.GetMarketSH()))},
			{Market: "SZ", State: strconv.Itoa(int(state.GetMarketSZ()))},
		},
	}
}

// ProgramStatusString converts OpenD's status enum to the stable diagnostic
// text exposed by the API.
func ProgramStatusString(status *commonpb.ProgramStatus) string {
	if status == nil {
		return "Unavailable"
	}
	value := status.GetType().String()
	if desc := status.GetStrExtDesc(); desc != "" {
		return value + ": " + desc
	}
	return value
}
