package futu

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	optionstrategypb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetoptionstrategy"
	optionanalysispb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetoptionstrategyanalysis"
	optionstrategyspreadpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetoptionstrategyspread"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdgetcombomaxpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetcombomaxtrdqtys"
)

func TestFutuOptionComboPreviewLegalityAndTransportFailures(t *testing.T) {
	server, adapter, intent := optionComboCoverageFixture(t)
	defer server.stop()
	ctx := t.Context()

	badLeg := intent
	badLeg.Legs = append([]broker.OrderLegIntent(nil), intent.Legs...)
	badLeg.Legs[0].InstrumentID = "BAD"
	if _, err := adapter.PreviewComboOrder(ctx, badLeg); err == nil {
		t.Fatal("combo with invalid leg symbol succeeded")
	}
	badSide := intent
	badSide.Legs = append([]broker.OrderLegIntent(nil), intent.Legs...)
	badSide.Legs[0].Side = "HOLD"
	if _, err := adapter.PreviewComboOrder(ctx, badSide); err == nil {
		t.Fatal("combo with invalid leg side succeeded")
	}
	badOwner := intent
	badOwner.UnderlyingID = "BAD"
	if _, err := adapter.PreviewComboOrder(ctx, badOwner); err == nil {
		t.Fatal("combo with invalid underlying succeeded")
	}

	noSpread := intent
	noSpread.Spread = nil
	if result, err := adapter.PreviewComboOrder(ctx, noSpread); err != nil ||
		result.ReasonCode != "INVALID_OPTION_SPREAD" {
		t.Fatalf("missing spread result = %#v, %v", result, err)
	}
	illegalValue := 5.0
	illegal := intent
	illegal.Spread = &illegalValue
	if result, err := adapter.PreviewComboOrder(ctx, illegal); err != nil ||
		result.ReasonCode != "ILLEGAL_OPTION_SPREAD" {
		t.Fatalf("illegal spread result = %#v, %v", result, err)
	}
	missingAccount := intent
	missingAccount.AccountID = "missing"
	if _, err := adapter.PreviewComboOrder(ctx, missingAccount); err == nil {
		t.Fatal("combo preview with missing account succeeded")
	}

	dropSpreadServer, dropSpread, dropSpreadIntent := optionComboCoverageFixture(t)
	defer dropSpreadServer.stop()
	dropSpreadServer.setDropProto(3258)
	if _, err := dropSpread.PreviewComboOrder(ctx, dropSpreadIntent); err == nil {
		t.Fatal("spread legality transport failure was hidden")
	}

	dropMaximumServer, dropMaximum, dropMaximumIntent := optionComboCoverageFixture(t)
	defer dropMaximumServer.stop()
	dropMaximumServer.setDropProto(opend.ProtoTrdGetComboMaxTrdQtys)
	if _, err := dropMaximum.PreviewComboOrder(ctx, dropMaximumIntent); err == nil {
		t.Fatal("combo buying-power transport failure was hidden")
	}

	dropAnalysisServer, dropAnalysis, dropAnalysisIntent := optionComboCoverageFixture(t)
	defer dropAnalysisServer.stop()
	dropAnalysisServer.setDropProto(3257)
	if _, err := dropAnalysis.PreviewComboOrder(ctx, dropAnalysisIntent); err == nil {
		t.Fatal("combo analysis transport failure was hidden")
	}
}

func TestFutuOptionComboNonSpreadOpenDLegalityBranches(t *testing.T) {
	server, adapter, intent := optionComboCoverageFixture(t)
	defer server.stop()
	ctx := t.Context()
	intent.OptionStrategy = "straddle"
	intent.Spread = nil
	legs, err := futuComboLegs(intent.Legs, false)
	if err != nil {
		t.Fatalf("combo legs: %v", err)
	}
	owner, _, err := futuSecurityFromSymbol(intent.UnderlyingID)
	if err != nil {
		t.Fatalf("combo owner: %v", err)
	}
	var clientResult *broker.ProductRuleResult
	err = adapter.exchange.withClient(ctx, func(client *opend.Client) error {
		clientResult, err = adapter.validateOptionComboLegality(
			ctx, client, intent, owner,
			int32(qotcommonpb.OptionStrategyType_OptionStrategyType_Straddle),
			legs,
		)
		return err
	})
	if err != nil || clientResult == nil ||
		clientResult.ReasonCode != "ILLEGAL_OPTION_COMBINATION" {
		t.Fatalf("illegal non-spread result = %#v, %v", clientResult, err)
	}

	server.setAdvancedResponse(3256, &optionstrategypb.Response{
		RetType: new(int32(0)),
		S2C: &optionstrategypb.S2C{StrategyList: []*optionstrategypb.OptionStrategyItem{{
			Code: new("straddle"), Name: new("Straddle"),
			OptionStrategy: new(int32(qotcommonpb.OptionStrategyType_OptionStrategyType_Straddle)),
			StockOwner:     owner, MultiLegs: legs,
		}}},
	})
	calendar := intent
	calendar.OptionStrategy = "calendar"
	calendar.FarExpiry = "2026-08-21"
	err = adapter.exchange.withClient(ctx, func(client *opend.Client) error {
		clientResult, err = adapter.validateOptionComboLegality(
			ctx, client, calendar, owner,
			int32(qotcommonpb.OptionStrategyType_OptionStrategyType_Straddle),
			legs,
		)
		return err
	})
	if err != nil || clientResult != nil {
		t.Fatalf("legal non-spread result = %#v, %v", clientResult, err)
	}

	dropServer, dropAdapter, dropIntent := optionComboCoverageFixture(t)
	defer dropServer.stop()
	dropServer.setDropProto(3256)
	dropIntent.OptionStrategy = "straddle"
	dropLegs, _ := futuComboLegs(dropIntent.Legs, false)
	dropOwner, _, _ := futuSecurityFromSymbol(dropIntent.UnderlyingID)
	if err := dropAdapter.exchange.withClient(ctx, func(client *opend.Client) error {
		_, callErr := dropAdapter.validateOptionComboLegality(
			ctx, client, dropIntent, dropOwner,
			int32(qotcommonpb.OptionStrategyType_OptionStrategyType_Straddle),
			dropLegs,
		)
		return callErr
	}); err == nil {
		t.Fatal("non-spread legality transport failure was hidden")
	}
}

func TestFutuOptionComboPreviewNormalizesAllAccountImpacts(t *testing.T) {
	server, adapter, intent := optionComboCoverageFixture(t)
	defer server.stop()
	server.setAdvancedResponse(opend.ProtoTrdGetComboMaxTrdQtys,
		&trdgetcombomaxpb.Response{
			RetType: new(int32(0)),
			S2C: &trdgetcombomaxpb.S2C{
				Header: &trdcommonpb.TrdHeader{
					TrdEnv:    new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
					AccID:     new(uint64(1001)),
					TrdMarket: new(int32(trdcommonpb.TrdMarket_TrdMarket_US)),
				},
				MaxTrdQtys: &trdcommonpb.ComboMaxTrdQtys{
					NlvChange: new(101.0), InitialMarginChange: new(12.0),
					MaintenanceMarginChange: new(8.0), OptionBuyPower: new(500.0),
					MaxWithDrawChange: new(-20.0), BuyPowerDecrease: new(30.0),
				},
			},
		},
	)
	server.setAdvancedResponse(3257, &optionanalysispb.Response{
		RetType: new(int32(0)),
		S2C: &optionanalysispb.S2C{
			Code: new("combo"), Name: new("Vertical"),
			OptionStrategy: new(int32(1)), Bid1: new(1.1), Ask1: new(1.3),
			MaxProfit: new(9_999_999.0), MaxLoss: new(250.0),
		},
	})

	result, err := adapter.PreviewComboOrder(t.Context(), intent)
	if err != nil {
		t.Fatalf("PreviewComboOrder: %v", err)
	}
	impact := result.AccountImpact
	if impact == nil || impact.NLVChange == nil || *impact.NLVChange != 101 ||
		impact.InitialMarginChange == nil || *impact.InitialMarginChange != 12 ||
		impact.MaintenanceMarginChange == nil || *impact.MaintenanceMarginChange != 8 ||
		impact.OptionBuyingPower == nil || *impact.OptionBuyingPower != 500 ||
		impact.MaxWithdrawalChange == nil || *impact.MaxWithdrawalChange != -20 ||
		impact.BuyingPowerDecrease == nil || *impact.BuyingPowerDecrease != 30 ||
		result.BuyingPowerImpact == nil || *result.BuyingPowerImpact != 30 {
		t.Fatalf("account impact = %#v, legacy=%v", impact, result.BuyingPowerImpact)
	}
	if result.OptionAnalysis == nil || !result.OptionAnalysis.MaxProfitUnlimited ||
		result.OptionAnalysis.MaxProfit != nil || result.OptionAnalysis.MaxLoss == nil ||
		*result.OptionAnalysis.MaxLoss != 250 {
		t.Fatalf("option analysis = %#v", result.OptionAnalysis)
	}
}

func TestFutuComboPlaceValidatedLegAccountAndTransportFailures(t *testing.T) {
	server, adapter, intent := optionComboCoverageFixture(t)
	defer server.stop()
	ctx := t.Context()

	badSide := intent
	badSide.Legs = append([]broker.OrderLegIntent(nil), intent.Legs...)
	badSide.Legs[0].Side = "HOLD"
	if _, err := adapter.PlaceComboOrder(ctx, badSide); err == nil {
		t.Fatal("placing invalid combo legs succeeded")
	}
	missingAccount := intent
	missingAccount.AccountID = "missing"
	if _, err := adapter.PlaceComboOrder(ctx, missingAccount); err == nil {
		t.Fatal("placing combo with missing account succeeded")
	}
	dropServer, dropAdapter, dropIntent := optionComboCoverageFixture(t)
	defer dropServer.stop()
	dropServer.setDropProto(opend.ProtoTrdPlaceComboOrder)
	if _, err := dropAdapter.PlaceComboOrder(ctx, dropIntent); err == nil {
		t.Fatal("combo place transport failure was hidden")
	}

	mixed := intent
	mixed.Legs = append([]broker.OrderLegIntent(nil), intent.Legs...)
	mixed.Legs[1].ProductClass = broker.ProductClassFuture
	if _, err := validateComboIntent(mixed); err == nil {
		t.Fatal("fully formed mixed-product combo succeeded")
	}
	badRatio := intent
	badRatio.Legs = append([]broker.OrderLegIntent(nil), intent.Legs...)
	badRatio.Legs[1].Ratio = 0
	if _, err := validateComboIntent(badRatio); err == nil {
		t.Fatal("fully formed zero-ratio combo succeeded")
	}
}

func optionComboCoverageFixture(
	t *testing.T,
) (*quoteOpenDServer, *futuAdapter, broker.ComboOrderIntent) {
	t.Helper()
	server := startQuoteOpenDServer(t)
	account := testSimulateHKCashAccount()
	account.TrdMarketAuthList = []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)}
	server.setAccounts([]*trdcommonpb.TrdAcc{account})
	spread := 10.0
	server.setAdvancedResponse(3258, &optionstrategyspreadpb.Response{
		RetType: new(int32(0)),
		S2C:     &optionstrategyspreadpb.S2C{SpreadList: []float64{spread}},
	})
	quantity := 2.0
	return server, newTestBrokerAdapter(t, server).(*futuAdapter), broker.ComboOrderIntent{
		ReadQuery: broker.ReadQuery{
			AccountID: "1001", Market: "US", TradingEnvironment: "SIMULATE",
		},
		ClientOrderID: "client", PreviewID: "preview",
		OrderKind: broker.OrderKindOptionCombo, ProductClass: broker.ProductClassOption,
		UnderlyingID: "US.AAPL", OptionStrategy: "vertical",
		NearExpiry: "2026-07-17", Spread: &spread,
		Legs: []broker.OrderLegIntent{
			{
				InstrumentID: "US.OPTION.ONE", ProductClass: broker.ProductClassOption,
				Side: "BUY", Ratio: 1, Quantity: &quantity,
			},
			{
				InstrumentID: "US.OPTION.TWO", ProductClass: broker.ProductClassOption,
				Side: "SELL", Ratio: 1, Quantity: &quantity,
			},
		},
	}
}
