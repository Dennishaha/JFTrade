package futu

import (
	"strings"
	"testing"

	productfeatures "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
	getuserinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/getuserinfo"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestFutuCapabilitiesContextLoadsInitialQuoteRightsOnce(t *testing.T) {
	tests := []struct {
		name       string
		response   *getuserinfopb.Response
		wantState  broker.CapabilityState
		wantCode   string
		wantReason string
	}{
		{
			name: "available",
			response: func() *getuserinfopb.Response {
				right := int32(qotcommonpb.QotRight_QotRight_Level2)
				return &getuserinfopb.Response{
					RetType: new(int32(0)),
					S2C: &getuserinfopb.S2C{
						UsQotRight: &right,
					},
				}
			}(),
			wantState: broker.CapabilityAvailable,
			wantCode:  "QUOTE_RIGHT_AVAILABLE",
		},
		{
			name: "query failure remains a successful capability response",
			response: &getuserinfopb.Response{
				RetType: new(int32(-1)),
				RetMsg:  new("permission query failed"),
			},
			wantState:  broker.CapabilityDegraded,
			wantCode:   "QUOTE_RIGHT_QUERY_FAILED",
			wantReason: "OpenD quote entitlement verification failed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := startQuoteOpenDServer(t)
			t.Cleanup(server.stop)
			server.setUserInfoResponse(test.response)

			adapter := newTestBrokerAdapter(t, server)
			registry := broker.NewRegistry()
			registry.Register(adapter)
			service := productfeatures.NewService(registry, adapter.ID(), nil, nil)

			result := service.CapabilitiesContext(t.Context(), productfeatures.CapabilityQuery{
				BrokerID: adapter.ID(),
				Market:   "US",
			})
			status := capabilityRuntimeStatus(
				t,
				result["runtime"].([]productfeatures.RuntimeCapabilityStatus),
				broker.FeatureMarketSnapshot,
			)
			if status.Capability.State != test.wantState ||
				status.Evaluation.State != test.wantState ||
				status.Evaluation.QuoteRight.Code != test.wantCode {
				t.Fatalf("market snapshot capability = %#v", status)
			}
			if test.wantReason != "" &&
				!strings.Contains(status.Evaluation.QuoteRight.Reason, test.wantReason) {
				t.Fatalf("quote-right reason = %q", status.Evaluation.QuoteRight.Reason)
			}
			if calls := server.userInfoCalls.Load(); calls != 1 {
				t.Fatalf("GetUserInfo calls = %d, want 1", calls)
			}
		})
	}
}

func capabilityRuntimeStatus(
	t testing.TB,
	statuses []productfeatures.RuntimeCapabilityStatus,
	featureID broker.FeatureID,
) productfeatures.RuntimeCapabilityStatus {
	t.Helper()
	for _, status := range statuses {
		if status.FeatureID == featureID {
			return status
		}
	}
	t.Fatalf("runtime status for %s not found in %#v", featureID, statuses)
	return productfeatures.RuntimeCapabilityStatus{}
}
