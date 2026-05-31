package jftradeapi

import (
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
)

func marketDataSecuritySnapshotFixture(security *qotcommonpb.Security, quoteAt time.Time) *qotgetsecuritysnapshotpb.Snapshot {
	code := strings.ToUpper(strings.TrimSpace(security.GetCode()))
	switch code {
	case "21164":
		return &qotgetsecuritysnapshotpb.Snapshot{
			Basic: marketDataSnapshotBasicFixture(security, quoteAt, "Tencent Bull 21164", qotcommonpb.SecurityType_SecurityType_Warrant, "2024-01-05", 10000, 0.001, 0.118, 0.115, 0.121, 0.113, 154000000, 18400000, 2.3),
			WarrantExData: &qotgetsecuritysnapshotpb.WarrantSnapshotExData{
				ConversionRate:     proto.Float64(10000),
				WarrantType:        proto.Int32(int32(qotcommonpb.WarrantType_WarrantType_Bull)),
				StrikePrice:        proto.Float64(320),
				MaturityTime:       proto.String("2026-12-30"),
				EndTradeTime:       proto.String("2026-12-29"),
				Owner:              marketDataProtoSecurity(qotcommonpb.QotMarket_QotMarket_HK_Security, "00700"),
				RecoveryPrice:      proto.Float64(300),
				StreetVolumn:       proto.Int64(32000000),
				IssueVolumn:        proto.Int64(64000000),
				StreetRate:         proto.Float64(50),
				Delta:              proto.Float64(0.48),
				ImpliedVolatility:  proto.Float64(28.5),
				Premium:            proto.Float64(12.6),
				MaturityTimestamp:  proto.Float64(1798588800),
				EndTradeTimestamp:  proto.Float64(1798502400),
				Leverage:           proto.Float64(8.2),
				Ipop:               proto.Float64(4.7),
				BreakEvenPoint:     proto.Float64(334.2),
				ConversionPrice:    proto.Float64(0.032),
				PriceRecoveryRatio: proto.Float64(6.8),
				Score:              proto.Float64(85),
				IssuerCode:         proto.String("SG"),
			},
		}
	case "AAPL250117C00200000":
		return &qotgetsecuritysnapshotpb.Snapshot{
			Basic: marketDataSnapshotBasicFixture(security, quoteAt, "AAPL 2025-01-17 200C", qotcommonpb.SecurityType_SecurityType_Drvt, "2024-08-01", 100, 0.01, 18.4, 18.1, 18.9, 17.7, 2300000, 42000000, 3.8),
			OptionExData: &qotgetsecuritysnapshotpb.OptionSnapshotExData{
				Type:                 proto.Int32(int32(qotcommonpb.OptionType_OptionType_Call)),
				Owner:                marketDataProtoSecurity(qotcommonpb.QotMarket_QotMarket_US_Security, "AAPL"),
				StrikeTime:           proto.String("2025-01-17"),
				StrikePrice:          proto.Float64(200),
				ContractSize:         proto.Int32(100),
				ContractSizeFloat:    proto.Float64(100),
				OpenInterest:         proto.Int32(14280),
				ImpliedVolatility:    proto.Float64(24.2),
				Premium:              proto.Float64(3.4),
				Delta:                proto.Float64(0.61),
				Gamma:                proto.Float64(0.02),
				Vega:                 proto.Float64(0.11),
				Theta:                proto.Float64(-0.08),
				Rho:                  proto.Float64(0.04),
				StrikeTimestamp:      proto.Float64(1737072000),
				ExpiryDateDistance:   proto.Int32(45),
				ContractNominalValue: proto.Float64(20000),
				OwnerLotMultiplier:   proto.Float64(1),
				ContractMultiplier:   proto.Float64(100),
			},
		}
	case "HSIMAIN":
		return &qotgetsecuritysnapshotpb.Snapshot{
			Basic: marketDataSnapshotBasicFixture(security, quoteAt, "HSI Main", qotcommonpb.SecurityType_SecurityType_Future, "2023-01-01", 50, 1, 18456, 18412, 18502, 18380, 92000, 1690000000, 0.7),
			FutureExData: &qotgetsecuritysnapshotpb.FutureSnapshotExData{
				LastSettlePrice:    proto.Float64(18390),
				Position:           proto.Int32(182233),
				PositionChange:     proto.Int32(4201),
				LastTradeTime:      proto.String("2026-06-29"),
				LastTradeTimestamp: proto.Float64(1782691200),
				IsMainContract:     proto.Bool(true),
			},
		}
	case "SPY":
		return &qotgetsecuritysnapshotpb.Snapshot{
			Basic: marketDataSnapshotBasicFixture(security, quoteAt, "SPDR S&P 500 ETF", qotcommonpb.SecurityType_SecurityType_Trust, "1993-01-22", 1, 0.01, 590.6, 589.2, 592.1, 587.4, 68000000, 40100000000, 0.92),
			TrustExData: &qotgetsecuritysnapshotpb.TrustSnapshotExData{
				DividendYield:    proto.Float64(1.3),
				Aum:              proto.Float64(580000000000),
				OutstandingUnits: proto.Int64(985000000),
				NetAssetValue:    proto.Float64(589.8),
				Premium:          proto.Float64(0.14),
				AssetClass:       proto.Int32(int32(qotcommonpb.AssetClass_AssetClass_Stock)),
			},
		}
	case "HSI":
		return &qotgetsecuritysnapshotpb.Snapshot{
			Basic: marketDataSnapshotBasicFixture(security, quoteAt, "Hang Seng Index", qotcommonpb.SecurityType_SecurityType_Index, "1969-11-24", 1, 0.01, 18456.2, 18398.1, 18512.4, 18354.6, 0, 0, 0),
			IndexExData: &qotgetsecuritysnapshotpb.IndexSnapshotExData{
				RaiseCount: proto.Int32(58),
				FallCount:  proto.Int32(21),
				EqualCount: proto.Int32(3),
			},
		}
	case "TECH":
		return &qotgetsecuritysnapshotpb.Snapshot{
			Basic: marketDataSnapshotBasicFixture(security, quoteAt, "Technology Sector", qotcommonpb.SecurityType_SecurityType_Plate, "2021-01-01", 1, 0.01, 7850.3, 7792.5, 7898.8, 7765.1, 0, 0, 0),
			PlateExData: &qotgetsecuritysnapshotpb.PlateSnapshotExData{
				RaiseCount: proto.Int32(42),
				FallCount:  proto.Int32(17),
				EqualCount: proto.Int32(5),
			},
		}
	default:
		return &qotgetsecuritysnapshotpb.Snapshot{
			Basic: marketDataSnapshotBasicFixture(security, quoteAt, "Tencent Holdings", qotcommonpb.SecurityType_SecurityType_Eqty, "2004-06-16", 100, 0.01, 321.4, 319.8, 322.6, 319.6, 1282100, 411020000, 1.25),
			EquityExData: &qotgetsecuritysnapshotpb.EquitySnapshotExData{
				IssuedShares:         proto.Int64(9600000000),
				IssuedMarketVal:      proto.Float64(3085440000000),
				NetAsset:             proto.Float64(950000000000),
				NetProfit:            proto.Float64(185000000000),
				EarningsPershare:     proto.Float64(19.2),
				OutstandingShares:    proto.Int64(9300000000),
				OutstandingMarketVal: proto.Float64(2989020000000),
				NetAssetPershare:     proto.Float64(98.3),
				EyRate:               proto.Float64(6.0),
				PeRate:               proto.Float64(16.7),
				PbRate:               proto.Float64(3.2),
				PeTTMRate:            proto.Float64(17.1),
				DividendTTM:          proto.Float64(3.4),
				DividendRatioTTM:     proto.Float64(1.1),
			},
		}
	}
}

func marketDataSnapshotBasicFixture(security *qotcommonpb.Security, quoteAt time.Time, name string, securityType qotcommonpb.SecurityType, listTime string, lotSize int32, priceSpread float64, currentPrice float64, openPrice float64, highPrice float64, lowPrice float64, volume int64, turnover float64, turnoverRate float64) *qotgetsecuritysnapshotpb.SnapshotBasicData {
	return &qotgetsecuritysnapshotpb.SnapshotBasicData{
		Security:            security,
		Name:                proto.String(name),
		Type:                proto.Int32(int32(securityType)),
		IsSuspend:           proto.Bool(false),
		ListTime:            proto.String(listTime),
		LotSize:             proto.Int32(lotSize),
		PriceSpread:         proto.Float64(priceSpread),
		UpdateTime:          proto.String(quoteAt.Format("2006-01-02 15:04:05")),
		HighPrice:           proto.Float64(highPrice),
		OpenPrice:           proto.Float64(openPrice),
		LowPrice:            proto.Float64(lowPrice),
		LastClosePrice:      proto.Float64(currentPrice * 0.97),
		CurPrice:            proto.Float64(currentPrice),
		Volume:              proto.Int64(volume),
		Turnover:            proto.Float64(turnover),
		TurnoverRate:        proto.Float64(turnoverRate),
		ListTimestamp:       proto.Float64(1087344000),
		UpdateTimestamp:     proto.Float64(float64(quoteAt.Unix())),
		AskPrice:            proto.Float64(currentPrice + priceSpread),
		BidPrice:            proto.Float64(currentPrice - priceSpread),
		AskVol:              proto.Int64(4000),
		BidVol:              proto.Int64(3800),
		Amplitude:           proto.Float64(2.5),
		AvgPrice:            proto.Float64(currentPrice * 0.99),
		BidAskRatio:         proto.Float64(12.3),
		VolumeRatio:         proto.Float64(1.1),
		Highest52WeeksPrice: proto.Float64(currentPrice * 1.24),
		Lowest52WeeksPrice:  proto.Float64(currentPrice * 0.81),
		HighestHistoryPrice: proto.Float64(currentPrice * 2.4),
		LowestHistoryPrice:  proto.Float64(currentPrice * 0.2),
		SecStatus:           proto.Int32(int32(qotcommonpb.SecurityStatus_SecurityStatus_Normal)),
		ClosePrice5Minute:   proto.Float64(currentPrice * 0.985),
		HpVolume:            proto.Float64(float64(volume)),
		HpAskVol:            proto.Float64(4000),
		HpBidVol:            proto.Float64(3800),
	}
}

func marketDataSecurityStaticInfoFixture(security *qotcommonpb.Security) *qotcommonpb.SecurityStaticInfo {
	code := strings.ToUpper(strings.TrimSpace(security.GetCode()))
	switch code {
	case "21164":
		return &qotcommonpb.SecurityStaticInfo{
			Basic: marketDataSecurityStaticBasicFixture(security, 21164, 10000, qotcommonpb.SecurityType_SecurityType_Warrant, "Tencent Bull 21164", "2024-01-05", qotcommonpb.ExchType_ExchType_HK_HKEX),
			WarrantExData: &qotcommonpb.WarrantStaticExData{
				Type:  proto.Int32(int32(qotcommonpb.WarrantType_WarrantType_Bull)),
				Owner: marketDataProtoSecurity(qotcommonpb.QotMarket_QotMarket_HK_Security, "00700"),
			},
		}
	case "AAPL250117C00200000":
		return &qotcommonpb.SecurityStaticInfo{
			Basic: marketDataSecurityStaticBasicFixture(security, 250117200, 100, qotcommonpb.SecurityType_SecurityType_Drvt, "AAPL 2025-01-17 200C", "2024-08-01", qotcommonpb.ExchType_ExchType_US_Option),
			OptionExData: &qotcommonpb.OptionStaticExData{
				Type:            proto.Int32(int32(qotcommonpb.OptionType_OptionType_Call)),
				Owner:           marketDataProtoSecurity(qotcommonpb.QotMarket_QotMarket_US_Security, "AAPL"),
				StrikeTime:      proto.String("2025-01-17"),
				StrikePrice:     proto.Float64(200),
				Suspend:         proto.Bool(false),
				Market:          proto.String("US"),
				StrikeTimestamp: proto.Float64(1737072000),
			},
		}
	case "HSIMAIN":
		return &qotcommonpb.SecurityStaticInfo{
			Basic: marketDataSecurityStaticBasicFixture(security, 990001, 50, qotcommonpb.SecurityType_SecurityType_Future, "HSI Main", "2023-01-01", qotcommonpb.ExchType_ExchType_HK_HKEX),
			FutureExData: &qotcommonpb.FutureStaticExData{
				LastTradeTime:      proto.String("2026-06-29"),
				LastTradeTimestamp: proto.Float64(1782691200),
				IsMainContract:     proto.Bool(true),
			},
		}
	case "SPY":
		return &qotcommonpb.SecurityStaticInfo{
			Basic: marketDataSecurityStaticBasicFixture(security, 500001, 1, qotcommonpb.SecurityType_SecurityType_Trust, "SPDR S&P 500 ETF", "1993-01-22", qotcommonpb.ExchType_ExchType_US_NYSE),
		}
	case "HSI":
		return &qotcommonpb.SecurityStaticInfo{
			Basic: marketDataSecurityStaticBasicFixture(security, 800001, 1, qotcommonpb.SecurityType_SecurityType_Index, "Hang Seng Index", "1969-11-24", qotcommonpb.ExchType_ExchType_HK_HKEX),
		}
	case "TECH":
		return &qotcommonpb.SecurityStaticInfo{
			Basic: marketDataSecurityStaticBasicFixture(security, 810001, 1, qotcommonpb.SecurityType_SecurityType_Plate, "Technology Sector", "2021-01-01", qotcommonpb.ExchType_ExchType_HK_HKEX),
		}
	default:
		return &qotcommonpb.SecurityStaticInfo{
			Basic: marketDataSecurityStaticBasicFixture(security, 700, 100, qotcommonpb.SecurityType_SecurityType_Eqty, "Tencent Holdings", "2004-06-16", qotcommonpb.ExchType_ExchType_HK_HKEX),
		}
	}
}

func marketDataSecurityStaticBasicFixture(security *qotcommonpb.Security, id int64, lotSize int32, securityType qotcommonpb.SecurityType, name string, listTime string, exchangeType qotcommonpb.ExchType) *qotcommonpb.SecurityStaticBasic {
	return &qotcommonpb.SecurityStaticBasic{
		Security:      security,
		Id:            proto.Int64(id),
		LotSize:       proto.Int32(lotSize),
		SecType:       proto.Int32(int32(securityType)),
		Name:          proto.String(name),
		ListTime:      proto.String(listTime),
		ListTimestamp: proto.Float64(1087344000),
		ExchType:      proto.Int32(int32(exchangeType)),
	}
}

func marketDataProtoSecurity(market qotcommonpb.QotMarket, code string) *qotcommonpb.Security {
	return &qotcommonpb.Security{
		Market: proto.Int32(int32(market)),
		Code:   proto.String(code),
	}
}
