package storage

import (
	"context"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/futu"
	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	globalpb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	historypb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotrequesthistorykl"
)

func TestSyncKLinesImmediateCancellation(t *testing.T) {
	store, err := NewFutuKLineStore(filepath.Join(t.TempDir(), "cancelled.db"))
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	queuedAt := time.Date(2026, time.May, 20, 9, 30, 0, 0, time.UTC)
	progress := NewSyncProgress("sync-cancelled", "HK.00700", queuedAt)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = store.SyncKLines(
		ctx,
		nil,
		"HK.00700",
		[]bbgotypes.Interval{bbgotypes.Interval1m},
		queuedAt,
		queuedAt.Add(3*time.Minute),
		qotcommonpb.RehabType_RehabType_Forward,
		klineSessionScopeLegacy,
		progress,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SyncKLines() error = %v, want context.Canceled", err)
	}

	snapshot := progress.Snapshot()
	if snapshot == nil {
		t.Fatal("expected cancellation snapshot")
	}
	if snapshot.Status != "cancelled" {
		t.Fatalf("cancelled sync status = %s", snapshot.Status)
	}
	if snapshot.TotalIntervals != 1 {
		t.Fatalf("cancelled sync total intervals = %d", snapshot.TotalIntervals)
	}
	if snapshot.CompletedIntervals != 0 {
		t.Fatalf("cancelled sync completed intervals = %d", snapshot.CompletedIntervals)
	}
	if snapshot.CompletedBatches != 0 {
		t.Fatalf("cancelled sync completed batches = %d", snapshot.CompletedBatches)
	}

	var rowCount int
	if err := store.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM `+quoteIdentifier(klineTableName("HK.00700", bbgotypes.Interval1m, RehabTypeName(int32(qotcommonpb.RehabType_RehabType_Forward))))).Scan(&rowCount); err == nil {
		t.Fatalf("expected no per-series table after cancelled sync, found %d rows", rowCount)
	} else if !strings.Contains(err.Error(), "no such table") {
		t.Fatalf("count rows after cancelled sync: %v", err)
	}
}

//nolint:funlen
func TestSyncKLinesCancellationAfterFirstBatch(t *testing.T) {
	prevPause := syncBatchPause
	prevBatchSize := syncBatchSize
	syncBatchPause = 50 * time.Millisecond
	syncBatchSize = 2
	t.Cleanup(func() {
		syncBatchPause = prevPause
		syncBatchSize = prevBatchSize
	})

	server := startSyncHistoryOpenDServer(t, [][]*qotcommonpb.KLine{{
		testSyncHistoryKLine(time.Date(2026, time.May, 20, 9, 31, 0, 0, time.UTC), 100),
	}})
	defer server.stop()

	store, err := NewFutuKLineStore(filepath.Join(t.TempDir(), "mid-cancel.db"))
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	exchange := futu.NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, exchange.Close()) }()

	since := time.Date(2026, time.May, 20, 9, 30, 0, 0, time.UTC)
	until := since.Add(5 * time.Minute)
	progress := NewSyncProgress("sync-mid-cancel", "HK.00700", since)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- store.SyncKLines(
			ctx,
			exchange,
			"HK.00700",
			[]bbgotypes.Interval{bbgotypes.Interval1m},
			since,
			until,
			qotcommonpb.RehabType_RehabType_Forward,
			klineSessionScopeLegacy,
			progress,
		)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		snapshot := progress.Snapshot()
		if snapshot != nil && snapshot.CompletedBatches == 1 {
			cancel()
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("timed out waiting for first completed batch")
		}
		time.Sleep(5 * time.Millisecond)
	}

	err = <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SyncKLines() error = %v, want context.Canceled", err)
	}

	snapshot := progress.Snapshot()
	if snapshot == nil {
		t.Fatal("expected mid-cancel snapshot")
	}
	if snapshot.Status != "cancelled" {
		t.Fatalf("mid-cancel status = %s, want cancelled", snapshot.Status)
	}
	if snapshot.CompletedIntervals != 0 {
		t.Fatalf("mid-cancel completed intervals = %d, want 0", snapshot.CompletedIntervals)
	}
	if snapshot.CompletedBatches != 1 {
		t.Fatalf("mid-cancel completed batches = %d, want 1", snapshot.CompletedBatches)
	}

	if got := server.historyCallCount(); got != 1 {
		t.Fatalf("history call count = %d, want 1", got)
	}

	var rowCount int
	tableName := quoteIdentifier(klineTableName("HK.00700", bbgotypes.Interval1m, RehabTypeName(int32(qotcommonpb.RehabType_RehabType_Forward))))
	if err := store.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM `+tableName).Scan(&rowCount); err != nil {
		t.Fatalf("count rows after mid-cancel sync: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("row count after mid-cancel sync = %d, want 1", rowCount)
	}
}

//nolint:funlen
func TestSyncKLinesSyncsAndSkipsCoveredBatch(t *testing.T) {
	prevPause := syncBatchPause
	syncBatchPause = 0
	t.Cleanup(func() {
		syncBatchPause = prevPause
	})

	server := startSyncHistoryOpenDServer(t, [][]*qotcommonpb.KLine{{
		testSyncHistoryKLine(time.Date(2026, time.May, 20, 9, 31, 0, 0, time.UTC), 100),
		testSyncHistoryKLine(time.Date(2026, time.May, 20, 9, 32, 0, 0, time.UTC), 101),
		testSyncHistoryKLine(time.Date(2026, time.May, 20, 9, 33, 0, 0, time.UTC), 102),
	}})
	defer server.stop()

	store, err := NewFutuKLineStore(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	exchange := futu.NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, exchange.Close()) }()

	since := time.Date(2026, time.May, 20, 9, 30, 0, 0, time.UTC)
	until := time.Date(2026, time.May, 20, 9, 33, 0, 0, time.UTC)

	firstProgress := NewSyncProgress("sync-1", "HK.00700", since)
	err = store.SyncKLines(
		context.Background(),
		exchange,
		"HK.00700",
		[]bbgotypes.Interval{bbgotypes.Interval1m},
		since,
		until,
		qotcommonpb.RehabType_RehabType_Forward,
		klineSessionScopeLegacy,
		firstProgress,
	)
	if err != nil {
		t.Fatalf("first SyncKLines() error = %v", err)
	}

	firstSnapshot := firstProgress.Snapshot()
	if firstSnapshot == nil {
		t.Fatal("expected first sync snapshot")
	}
	if firstSnapshot.Status != "completed" {
		t.Fatalf("first sync status = %s", firstSnapshot.Status)
	}
	if firstSnapshot.CompletedIntervals != 1 {
		t.Fatalf("first sync completed intervals = %d", firstSnapshot.CompletedIntervals)
	}
	if firstSnapshot.CompletedBatches == 0 {
		t.Fatal("expected completed batches to be recorded")
	}

	var rowCount int
	tableName := quoteIdentifier(klineTableName("HK.00700", bbgotypes.Interval1m, RehabTypeName(int32(qotcommonpb.RehabType_RehabType_Forward))))
	if err := store.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM `+tableName).Scan(&rowCount); err != nil {
		t.Fatalf("count synced rows: %v", err)
	}
	if rowCount != 3 {
		t.Fatalf("synced row count = %d, want 3", rowCount)
	}

	firstCalls := server.historyCallCount()
	if firstCalls != 1 {
		t.Fatalf("history call count after first sync = %d, want 1", firstCalls)
	}

	secondProgress := NewSyncProgress("sync-2", "HK.00700", since)
	err = store.SyncKLines(
		context.Background(),
		exchange,
		"HK.00700",
		[]bbgotypes.Interval{bbgotypes.Interval1m},
		since,
		until,
		qotcommonpb.RehabType_RehabType_Forward,
		klineSessionScopeLegacy,
		secondProgress,
	)
	if err != nil {
		t.Fatalf("second SyncKLines() error = %v", err)
	}

	secondSnapshot := secondProgress.Snapshot()
	if secondSnapshot == nil {
		t.Fatal("expected second sync snapshot")
	}
	if secondSnapshot.Status != "completed" {
		t.Fatalf("second sync status = %s", secondSnapshot.Status)
	}
	if secondSnapshot.CompletedIntervals != 1 {
		t.Fatalf("second sync completed intervals = %d", secondSnapshot.CompletedIntervals)
	}
	if secondSnapshot.CompletedBatches == 0 {
		t.Fatal("expected second sync to mark a covered batch")
	}

	secondCalls := server.historyCallCount()
	if secondCalls != firstCalls {
		t.Fatalf("expected covered batch to skip RequestHistoryKL, calls before=%d after=%d", firstCalls, secondCalls)
	}

	if err := store.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM `+tableName).Scan(&rowCount); err != nil {
		t.Fatalf("count rows after second sync: %v", err)
	}
	if rowCount != 3 {
		t.Fatalf("row count after second sync = %d, want 3", rowCount)
	}
}

func TestSyncKLinesPersistentRateLimitFailure(t *testing.T) {
	prevPause := syncBatchPause
	prevBaseDelay := syncRetryBaseDelay
	syncBatchPause = 0
	syncRetryBaseDelay = 0
	t.Cleanup(func() {
		syncBatchPause = prevPause
		syncRetryBaseDelay = prevBaseDelay
	})

	server := startSyncHistoryOpenDServer(t, nil)
	server.setHistoryErrorResponse(&historypb.Response{
		RetType: new(int32(-1)),
		RetMsg:  new("频率太高"),
	})
	defer server.stop()

	store, err := NewFutuKLineStore(filepath.Join(t.TempDir(), "rate-limit.db"))
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	exchange := futu.NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, exchange.Close()) }()

	queuedAt := time.Date(2026, time.May, 20, 9, 30, 0, 0, time.UTC)
	progress := NewSyncProgress("sync-rate-limit", "HK.00700", queuedAt)
	err = store.SyncKLines(
		context.Background(),
		exchange,
		"HK.00700",
		[]bbgotypes.Interval{bbgotypes.Interval1m},
		queuedAt,
		queuedAt.Add(3*time.Minute),
		qotcommonpb.RehabType_RehabType_Forward,
		klineSessionScopeLegacy,
		progress,
	)
	if err == nil {
		t.Fatal("expected persistent rate-limit failure")
	}
	if !strings.Contains(err.Error(), "retry exhausted after 3 retries") {
		t.Fatalf("expected retry exhaustion error, got %v", err)
	}
	if !strings.Contains(err.Error(), "retType=-1") {
		t.Fatalf("expected OpenD retType in error, got %v", err)
	}

	snapshot := progress.Snapshot()
	if snapshot == nil {
		t.Fatal("expected rate-limit snapshot")
	}
	if snapshot.Status != "failed" {
		t.Fatalf("rate-limit status = %s, want failed", snapshot.Status)
	}
	if snapshot.Retries != 3 {
		t.Fatalf("rate-limit retries = %d, want 3", snapshot.Retries)
	}
	if !strings.Contains(snapshot.Error, "retry exhausted after 3 retries") {
		t.Fatalf("unexpected progress error = %s", snapshot.Error)
	}
	if snapshot.CompletedBatches != 0 {
		t.Fatalf("rate-limit completed batches = %d, want 0", snapshot.CompletedBatches)
	}

	if got := server.historyCallCount(); got != 4 {
		t.Fatalf("history call count = %d, want 4", got)
	}

	var rowCount int
	tableName := quoteIdentifier(klineTableName("HK.00700", bbgotypes.Interval1m, RehabTypeName(int32(qotcommonpb.RehabType_RehabType_Forward))))
	if err := store.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM `+tableName).Scan(&rowCount); err == nil {
		if rowCount != 0 {
			t.Fatalf("row count after rate-limit sync = %d, want 0", rowCount)
		}
		return
	} else if !strings.Contains(err.Error(), "no such table") {
		t.Fatalf("count rows after rate-limit sync: %v", err)
	}
}

type syncHistoryOpenDServer struct {
	addr              string
	listener          net.Listener
	stopOnce          sync.Once
	shutdownCompleted chan struct{}
	historyCalls      atomic.Int32
	historyError      *historypb.Response
	pages             [][]*qotcommonpb.KLine
	pagesMu           sync.Mutex
}

func startSyncHistoryOpenDServer(t *testing.T, pages [][]*qotcommonpb.KLine) *syncHistoryOpenDServer {
	t.Helper()

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	server := &syncHistoryOpenDServer{
		addr:              listener.Addr().String(),
		listener:          listener,
		shutdownCompleted: make(chan struct{}),
		pages:             cloneSyncHistoryPages(pages),
	}
	go server.acceptLoop()
	return server
}

func (s *syncHistoryOpenDServer) stop() {
	s.stopOnce.Do(func() {
		jftradeErr1 := s.listener.Close()
		jftradePanicOnError(jftradeErr1)
		<-s.shutdownCompleted
	})
}

func (s *syncHistoryOpenDServer) historyCallCount() int {
	return int(s.historyCalls.Load())
}

func (s *syncHistoryOpenDServer) setHistoryErrorResponse(response *historypb.Response) {
	s.pagesMu.Lock()
	defer s.pagesMu.Unlock()
	s.historyError = response
}

func (s *syncHistoryOpenDServer) acceptLoop() {
	defer close(s.shutdownCompleted)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *syncHistoryOpenDServer) handleConn(conn net.Conn) {
	defer func() { jftradePanicOnError(conn.Close()) }()
	for {
		header := make([]byte, codec.HeaderLen)
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		bodyLen := int(uint32(header[12]) | uint32(header[13])<<8 | uint32(header[14])<<16 | uint32(header[15])<<24)
		packet := make([]byte, codec.HeaderLen+bodyLen)
		copy(packet, header)
		if _, err := io.ReadFull(conn, packet[codec.HeaderLen:]); err != nil {
			return
		}

		frame, err := codec.Decode(packet)
		if err != nil {
			return
		}

		var response proto.Message
		switch frame.Header.ProtoID {
		case opend.ProtoInitConnect:
			response = &initpb.Response{
				RetType: new(int32(0)),
				S2C: &initpb.S2C{
					ServerVer:         new(int32(1009)),
					LoginUserID:       new(uint64(1)),
					ConnID:            new(uint64(42)),
					ConnAESKey:        new("0123456789abcdef"),
					KeepAliveInterval: new(int32(10)),
				},
			}
		case opend.ProtoGetGlobalState:
			response = syncHistoryGlobalStateResponse()
		case opend.ProtoRequestHistoryKL:
			response = s.historyKLResponse(frame.Body)
		default:
			return
		}

		body, err := proto.Marshal(response)
		if err != nil {
			return
		}
		packet, err = codec.Encode(frame.Header.ProtoID, frame.Header.SerialNo, body)
		if err != nil {
			return
		}
		if _, err := conn.Write(packet); err != nil {
			return
		}
	}
}

func syncHistoryGlobalStateResponse() *globalpb.Response {
	zero := int32(0)
	return &globalpb.Response{
		RetType: new(int32(0)),
		S2C: &globalpb.S2C{
			MarketHK:       &zero,
			MarketUS:       &zero,
			MarketSH:       &zero,
			MarketSZ:       &zero,
			MarketHKFuture: &zero,
			QotLogined:     new(true),
			TrdLogined:     new(true),
			ServerVer:      new(int32(1009)),
			ServerBuildNo:  new(int32(6908)),
			Time:           new(int64(0)),
		},
	}
}

func (s *syncHistoryOpenDServer) historyKLResponse(body []byte) *historypb.Response {
	request := &historypb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &historypb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}

	s.historyCalls.Add(1)
	s.pagesMu.Lock()
	defer s.pagesMu.Unlock()
	if s.historyError != nil {
		return jftradeCheckedTypeAssertion[*historypb.Response](proto.Clone(s.historyError))
	}

	response := &historypb.Response{
		RetType: new(int32(0)),
		S2C: &historypb.S2C{
			Security: request.GetC2S().GetSecurity(),
		},
	}

	if len(s.pages) == 0 {
		return response
	}

	pageIndex := max(int(s.historyCalls.Load())-1, 0)
	if pageIndex >= len(s.pages) {
		pageIndex = len(s.pages) - 1
	}
	response.S2C.KlList = s.pages[pageIndex]
	if pageIndex < len(s.pages)-1 {
		response.S2C.NextReqKey = []byte{byte(pageIndex + 1)}
	}
	return response
}

func cloneSyncHistoryPages(pages [][]*qotcommonpb.KLine) [][]*qotcommonpb.KLine {
	cloned := make([][]*qotcommonpb.KLine, 0, len(pages))
	for _, page := range pages {
		cloned = append(cloned, append([]*qotcommonpb.KLine(nil), page...))
	}
	return cloned
}

func testSyncHistoryKLine(at time.Time, price float64) *qotcommonpb.KLine {
	return &qotcommonpb.KLine{
		Time:       new(at.Format("2006-01-02 15:04:05")),
		Timestamp:  new(float64(at.Unix())),
		IsBlank:    new(false),
		OpenPrice:  new(price),
		HighPrice:  new(price + 1),
		LowPrice:   new(price - 1),
		ClosePrice: new(price + 0.5),
		Volume:     new(int64(1000)),
		Turnover:   new(price * 1000),
	}
}

func jftradeCheckTestError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func jftradePanicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
