package futu

import (
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
)

func TestSecuritySnapshotReadersHandleDuplicateMissingAndTransportBoundaries(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	server.setSecuritySnapshots([]*qotgetsecuritysnapshotpb.Snapshot{testTencentSecuritySnapshot()})
	server.setStaticInfos([]*qotcommonpb.SecurityStaticInfo{testTencentStaticInfo()})

	snapshots, err := exchange.querySecuritySnapshotList(t.Context(), []string{"HK.00700", " hk.00700 "})
	if err != nil || len(snapshots) != 1 || snapshots["HK.00700"] == nil {
		t.Fatalf("querySecuritySnapshotList(duplicates) = %#v, %v", snapshots, err)
	}
	infos, err := exchange.queryStaticInfoList(t.Context(), []string{"HK.00700", " hk.00700 "})
	if err != nil || len(infos) != 1 || infos["HK.00700"] == nil {
		t.Fatalf("queryStaticInfoList(duplicates) = %#v, %v", infos, err)
	}

	if _, err := exchange.querySecuritySnapshot(t.Context(), "US.AAPL"); err == nil || !strings.Contains(err.Error(), "no snapshot") {
		t.Fatalf("querySecuritySnapshot(missing requested symbol) error = %v", err)
	}
	if _, err := exchange.queryStaticInfo(t.Context(), "US.AAPL"); err == nil || !strings.Contains(err.Error(), "no static info") {
		t.Fatalf("queryStaticInfo(missing requested symbol) error = %v", err)
	}

	server.setSecuritySnapshots(nil)
	resetSecuritySnapshotCoordinator(exchange)
	if _, err := exchange.querySecuritySnapshotList(t.Context(), []string{"HK.00700"}); err == nil || !strings.Contains(err.Error(), "no snapshots") {
		t.Fatalf("querySecuritySnapshotList(empty response) error = %v", err)
	}
	server.setStaticInfos(nil)
	if _, err := exchange.queryStaticInfoList(t.Context(), []string{"HK.00700"}); err == nil || !strings.Contains(err.Error(), "no static info") {
		t.Fatalf("queryStaticInfoList(empty response) error = %v", err)
	}

	deadExchange := NewExchangeWithConfig(opend.Config{Addr: "127.0.0.1:1", RequestTimeout: 50 * time.Millisecond})
	defer func() { jftradeCheckTestError(t, deadExchange.Close()) }()
	if _, err := deadExchange.querySecuritySnapshotList(t.Context(), []string{"HK.00700"}); err == nil {
		t.Fatal("querySecuritySnapshotList(unreachable OpenD) error = nil")
	}
	if _, err := deadExchange.queryStaticInfoList(t.Context(), []string{"HK.00700"}); err == nil {
		t.Fatal("queryStaticInfoList(unreachable OpenD) error = nil")
	}
}

func TestMergeStaticInfoFillsAbsentSnapshotFieldsWithoutOverwritingExistingValues(t *testing.T) {
	details := &SecurityDetails{}
	mergeStaticInfoIntoSecurityDetails(details, testTencentStaticInfo())
	if details.Name != "Tencent" || details.SecurityType != "Eqty" || details.LotSize != 100 || details.SecurityID == nil || *details.SecurityID != 700 {
		t.Fatalf("merged empty security details = %#v", details)
	}

	mergeStaticInfoIntoSecurityDetails(nil, testTencentStaticInfo())
	mergeStaticInfoIntoSecurityDetails(details, nil)
	mergeStaticInfoIntoSecurityDetails(details, &qotcommonpb.SecurityStaticInfo{})
}
