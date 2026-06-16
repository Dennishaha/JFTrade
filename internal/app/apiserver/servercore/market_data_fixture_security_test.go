package servercore

import (
	"strings"
	"time"

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
				ConversionRate:     new(float64(10000)),
				WarrantType:        new(int32(qotcommonpb.WarrantType_WarrantType_Bull)),
				StrikePrice:        new(float64(320)),
				MaturityTime:       new("2026-12-30"),
				EndTradeTime:       new("2026-12-29"),
				Owner:              marketDataProtoSecurity(qotcommonpb.QotMarket_QotMarket_HK_Security, "00700"),
				RecoveryPrice:      new(float64(300)),
				StreetVolumn:       new(int64(32000000)),
				IssueVolumn:        new(int64(64000000)),
				StreetRate:         new(float64(50)),
				Delta:              new(0.48),
				ImpliedVolatility:  new(28.5),
				Premium:            new(12.6),
				MaturityTimestamp:  new(float64(1798588800)),
				EndTradeTimestamp:  new(float64(1798502400)),
				Leverage:           new(8.2),
				Ipop:               new(4.7),
				BreakEvenPoint:     new(334.2),
				ConversionPrice:    new(0.032),
				PriceRecoveryRatio: new(6.8),
				Score:              new(float64(85)),
				IssuerCode:         new("SG"),
			},
		}
	case "AAPL250117C00200000":
		return &qotgetsecuritysnapshotpb.Snapshot{
			Basic: marketDataSnapshotBasicFixture(security, quoteAt, "AAPL 2025-01-17 200C", qotcommonpb.SecurityType_SecurityType_Drvt, "2024-08-01", 100, 0.01, 18.4, 18.1, 18.9, 17.7, 2300000, 42000000, 3.8),
			OptionExData: &qotgetsecuritysnapshotpb.OptionSnapshotExData{
				Type:                 new(int32(qotcommonpb.OptionType_OptionType_Call)),
				Owner:                marketDataProtoSecurity(qotcommonpb.QotMarket_QotMarket_US_Security, "AAPL"),
				StrikeTime:           new("2025-01-17"),
				StrikePrice:          new(float64(200)),
				ContractSize:         new(int32(100)),
				ContractSizeFloat:    new(float64(100)),
				OpenInterest:         new(int32(14280)),
				ImpliedVolatility:    new(24.2),
				Premium:              new(3.4),
				Delta:                new(0.61),
				Gamma:                new(0.02),
				Vega:                 new(0.11),
				Theta:                new(-0.08),
				Rho:                  new(0.04),
				StrikeTimestamp:      new(float64(1737072000)),
				ExpiryDateDistance:   new(int32(45)),
				ContractNominalValue: new(float64(20000)),
				OwnerLotMultiplier:   new(float64(1)),
				ContractMultiplier:   new(float64(100)),
			},
		}
	case "HSIMAIN":
		return &qotgetsecuritysnapshotpb.Snapshot{
			Basic: marketDataSnapshotBasicFixture(security, quoteAt, "HSI Main", qotcommonpb.SecurityType_SecurityType_Future, "2023-01-01", 50, 1, 18456, 18412, 18502, 18380, 92000, 1690000000, 0.7),
			FutureExData: &qotgetsecuritysnapshotpb.FutureSnapshotExData{
				LastSettlePrice:    new(float64(18390)),
				Position:           new(int32(182233)),
				PositionChange:     new(int32(4201)),
				LastTradeTime:      new("2026-06-29"),
				LastTradeTimestamp: new(float64(1782691200)),
				IsMainContract:     new(true),
			},
		}
	case "SPY":
		return &qotgetsecuritysnapshotpb.Snapshot{
			Basic: marketDataSnapshotBasicFixture(security, quoteAt, "SPDR S&P 500 ETF", qotcommonpb.SecurityType_SecurityType_Trust, "1993-01-22", 1, 0.01, 590.6, 589.2, 592.1, 587.4, 68000000, 40100000000, 0.92),
			TrustExData: &qotgetsecuritysnapshotpb.TrustSnapshotExData{
				DividendYield:    new(1.3),
				Aum:              new(float64(580000000000)),
				OutstandingUnits: new(int64(985000000)),
				NetAssetValue:    new(589.8),
				Premium:          new(0.14),
				AssetClass:       new(int32(qotcommonpb.AssetClass_AssetClass_Stock)),
			},
		}
	case "HSI":
		return &qotgetsecuritysnapshotpb.Snapshot{
			Basic: marketDataSnapshotBasicFixture(security, quoteAt, "Hang Seng Index", qotcommonpb.SecurityType_SecurityType_Index, "1969-11-24", 1, 0.01, 18456.2, 18398.1, 18512.4, 18354.6, 0, 0, 0),
			IndexExData: &qotgetsecuritysnapshotpb.IndexSnapshotExData{
				RaiseCount: new(int32(58)),
				FallCount:  new(int32(21)),
				EqualCount: new(int32(3)),
			},
		}
	case "TECH":
		return &qotgetsecuritysnapshotpb.Snapshot{
			Basic: marketDataSnapshotBasicFixture(security, quoteAt, "Technology Sector", qotcommonpb.SecurityType_SecurityType_Plate, "2021-01-01", 1, 0.01, 7850.3, 7792.5, 7898.8, 7765.1, 0, 0, 0),
			PlateExData: &qotgetsecuritysnapshotpb.PlateSnapshotExData{
				RaiseCount: new(int32(42)),
				FallCount:  new(int32(17)),
				EqualCount: new(int32(5)),
			},
		}
	default:
		return &qotgetsecuritysnapshotpb.Snapshot{
			Basic: marketDataSnapshotBasicFixture(security, quoteAt, "Tencent Holdings", qotcommonpb.SecurityType_SecurityType_Eqty, "2004-06-16", 100, 0.01, 321.4, 319.8, 322.6, 319.6, 1282100, 411020000, 1.25),
			EquityExData: &qotgetsecuritysnapshotpb.EquitySnapshotExData{
				IssuedShares:         new(int64(9600000000)),
				IssuedMarketVal:      new(float64(3085440000000)),
				NetAsset:             new(float64(950000000000)),
				NetProfit:            new(float64(185000000000)),
				EarningsPershare:     new(19.2),
				OutstandingShares:    new(int64(9300000000)),
				OutstandingMarketVal: new(float64(2989020000000)),
				NetAssetPershare:     new(98.3),
				EyRate:               new(6.0),
				PeRate:               new(16.7),
				PbRate:               new(3.2),
				PeTTMRate:            new(17.1),
				DividendTTM:          new(3.4),
				DividendRatioTTM:     new(1.1),
			},
		}
	}
}

func marketDataSnapshotBasicFixture(security *qotcommonpb.Security, quoteAt time.Time, name string, securityType qotcommonpb.SecurityType, listTime string, lotSize int32, priceSpread float64, currentPrice float64, openPrice float64, highPrice float64, lowPrice float64, volume int64, turnover float64, turnoverRate float64) *qotgetsecuritysnapshotpb.SnapshotBasicData {
	return &qotgetsecuritysnapshotpb.SnapshotBasicData{
		Security:            security,
		Name:                new(name),
		Type:                new(int32(securityType)),
		IsSuspend:           new(false),
		ListTime:            new(listTime),
		LotSize:             new(lotSize),
		PriceSpread:         new(priceSpread),
		UpdateTime:          new(quoteAt.Format("2006-01-02 15:04:05")),
		HighPrice:           new(highPrice),
		OpenPrice:           new(openPrice),
		LowPrice:            new(lowPrice),
		LastClosePrice:      new(currentPrice * 0.97),
		CurPrice:            new(currentPrice),
		Volume:              new(volume),
		Turnover:            new(turnover),
		TurnoverRate:        new(turnoverRate),
		ListTimestamp:       new(float64(1087344000)),
		UpdateTimestamp:     new(float64(quoteAt.Unix())),
		AskPrice:            new(currentPrice + priceSpread),
		BidPrice:            new(currentPrice - priceSpread),
		AskVol:              new(int64(4000)),
		BidVol:              new(int64(3800)),
		Amplitude:           new(2.5),
		AvgPrice:            new(currentPrice * 0.99),
		BidAskRatio:         new(12.3),
		VolumeRatio:         new(1.1),
		Highest52WeeksPrice: new(currentPrice * 1.24),
		Lowest52WeeksPrice:  new(currentPrice * 0.81),
		HighestHistoryPrice: new(currentPrice * 2.4),
		LowestHistoryPrice:  new(currentPrice * 0.2),
		SecStatus:           new(int32(qotcommonpb.SecurityStatus_SecurityStatus_Normal)),
		ClosePrice5Minute:   new(currentPrice * 0.985),
		HpVolume:            new(float64(volume)),
		HpAskVol:            new(float64(4000)),
		HpBidVol:            new(float64(3800)),
	}
}

func marketDataSecurityStaticInfoFixture(security *qotcommonpb.Security) *qotcommonpb.SecurityStaticInfo {
	code := strings.ToUpper(strings.TrimSpace(security.GetCode()))
	switch code {
	case "21164":
		return &qotcommonpb.SecurityStaticInfo{
			Basic: marketDataSecurityStaticBasicFixture(security, 21164, 10000, qotcommonpb.SecurityType_SecurityType_Warrant, "Tencent Bull 21164", "2024-01-05", qotcommonpb.ExchType_ExchType_HK_HKEX),
			WarrantExData: &qotcommonpb.WarrantStaticExData{
				Type:  new(int32(qotcommonpb.WarrantType_WarrantType_Bull)),
				Owner: marketDataProtoSecurity(qotcommonpb.QotMarket_QotMarket_HK_Security, "00700"),
			},
		}
	case "AAPL250117C00200000":
		return &qotcommonpb.SecurityStaticInfo{
			Basic: marketDataSecurityStaticBasicFixture(security, 250117200, 100, qotcommonpb.SecurityType_SecurityType_Drvt, "AAPL 2025-01-17 200C", "2024-08-01", qotcommonpb.ExchType_ExchType_US_Option),
			OptionExData: &qotcommonpb.OptionStaticExData{
				Type:            new(int32(qotcommonpb.OptionType_OptionType_Call)),
				Owner:           marketDataProtoSecurity(qotcommonpb.QotMarket_QotMarket_US_Security, "AAPL"),
				StrikeTime:      new("2025-01-17"),
				StrikePrice:     new(float64(200)),
				Suspend:         new(false),
				Market:          new("US"),
				StrikeTimestamp: new(float64(1737072000)),
			},
		}
	case "HSIMAIN":
		return &qotcommonpb.SecurityStaticInfo{
			Basic: marketDataSecurityStaticBasicFixture(security, 990001, 50, qotcommonpb.SecurityType_SecurityType_Future, "HSI Main", "2023-01-01", qotcommonpb.ExchType_ExchType_HK_HKEX),
			FutureExData: &qotcommonpb.FutureStaticExData{
				LastTradeTime:      new("2026-06-29"),
				LastTradeTimestamp: new(float64(1782691200)),
				IsMainContract:     new(true),
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
		Id:            new(id),
		LotSize:       new(lotSize),
		SecType:       new(int32(securityType)),
		Name:          new(name),
		ListTime:      new(listTime),
		ListTimestamp: new(float64(1087344000)),
		ExchType:      new(int32(exchangeType)),
	}
}

func marketDataProtoSecurity(market qotcommonpb.QotMarket, code string) *qotcommonpb.Security {
	return &qotcommonpb.Security{
		Market: new(int32(market)),
		Code:   new(code),
	}
}
