// Package opend is a small TCP client for Futu OpenD, providing a
// request/response API on top of the codec frame layer.
package opend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	keepalivepb "github.com/jftrade/jftrade-main/pkg/futu/pb/keepalive"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
)

// Protocol IDs (selected — see Futu OpenAPI docs for the full list).
const (
	ProtoInitConnect         uint32 = 1001
	ProtoGetGlobalState      uint32 = 1002
	ProtoNotify              uint32 = 1003
	ProtoKeepAlive           uint32 = 1004
	ProtoQotSub              uint32 = 3001
	ProtoGetBasicQot         uint32 = 3004
	ProtoQotUpdateBasicQot   uint32 = 3005
	ProtoGetKL               uint32 = 3006
	ProtoGetStaticInfo       uint32 = 3007
	ProtoGetSecuritySnapshot uint32 = 3203
	ProtoRequestHistoryKL    uint32 = 3103

	ProtoTrdGetAccList              uint32 = 2001
	ProtoTrdUnlockTrade             uint32 = 2005
	ProtoTrdSubAccPush              uint32 = 2008
	ProtoTrdGetFunds                uint32 = 2101
	ProtoTrdGetPositionList         uint32 = 2102
	ProtoTrdGetOrderList            uint32 = 2201
	ProtoTrdPlaceOrder              uint32 = 2202
	ProtoTrdModifyOrder             uint32 = 2205
	ProtoTrdGetHistoryOrderList     uint32 = 2221
	ProtoTrdGetHistoryOrderFillList uint32 = 2223
	ProtoTrdUpdateOrder             uint32 = 2208
	ProtoTrdUpdateOrderFill         uint32 = 2218
)

// Config controls how the client connects to OpenD.
type Config struct {
	// Addr is the native OpenD API host:port, e.g. "127.0.0.1:11110".
	Addr string
	// WebSocketKey is used by FTWebSocket / JavaScript API. The native OpenD TCP
	// API does not use it, but we keep the field so the same settings payload can
	// feed both API and FTWebSocket diagnostics.
	WebSocketKey string
	// TLS is not used by the native OpenD TCP API. It is kept for backward
	// compatibility with older call sites.
	TLS bool
	// HandshakeTimeout caps the TCP dial timeout (default 10s).
	HandshakeTimeout time.Duration
	// RequestTimeout caps a single RPC (default 15s).
	RequestTimeout time.Duration
}

// Errors.
var (
	ErrClosed         = errors.New("opend: client closed")
	ErrRequestTimeout = errors.New("opend: request timed out")
)

type pending struct {
	ch chan codec.Frame
}

// Client is a Futu OpenD client. Safe for concurrent use after Connect.
type Client struct {
	cfg Config

	conn   net.Conn
	serial uint32
	connID atomic.Uint64
	packet uint32

	mu            sync.Mutex
	pend          map[uint32]*pending
	subs          map[uint32][]func(codec.Frame)
	closed        bool
	writeMu       sync.Mutex
	done          chan struct{}
	doneOnce      sync.Once
	keepAliveOnce sync.Once
}

// New creates a Client; Connect must be called to dial.
func New(cfg Config) *Client {
	if cfg.HandshakeTimeout == 0 {
		cfg.HandshakeTimeout = 10 * time.Second
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = 15 * time.Second
	}
	return &Client{cfg: cfg, pend: map[uint32]*pending{}, subs: map[uint32][]func(codec.Frame){}, done: make(chan struct{})}
}

// Connect dials the native OpenD TCP API and starts the read loop.
func (c *Client) Connect(ctx context.Context) error {
	d := &net.Dialer{Timeout: c.cfg.HandshakeTimeout}
	conn, err := d.DialContext(ctx, "tcp", c.cfg.Addr)
	if err != nil {
		return fmt.Errorf("opend: dial %s: %w", c.cfg.Addr, err)
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = conn.Close()
		return ErrClosed
	}
	c.conn = conn
	c.mu.Unlock()
	go c.readLoop(conn)
	return nil
}

// Close terminates the connection.
func (c *Client) Close() error {
	return c.closeConn(true)
}

// Done is closed when the underlying OpenD session is closed or lost.
func (c *Client) Done() <-chan struct{} {
	return c.done
}

func (c *Client) SetConnID(connID uint64) {
	c.connID.Store(connID)
}

func (c *Client) ConnID() uint64 {
	return c.connID.Load()
}

func (c *Client) NextPacketID() *commonpb.PacketID {
	connID := c.ConnID()
	if connID == 0 {
		return nil
	}
	serialNo := atomic.AddUint32(&c.packet, 1)
	return &commonpb.PacketID{
		ConnID:   proto.Uint64(connID),
		SerialNo: proto.Uint32(serialNo),
	}
}

// StartKeepAlive sends Futu KeepAlive packets until the session closes.  OpenD
// advertises the required interval in InitConnect.S2C.keepAliveInterval; missing
// heartbeats can leave the TCP session accepted but unresponsive until OpenD is
// restarted.
func (c *Client) StartKeepAlive(interval time.Duration) {
	if interval <= 0 {
		return
	}
	c.keepAliveOnce.Do(func() {
		go c.keepAliveLoop(interval)
	})
}

func (c *Client) keepAliveLoop(interval time.Duration) {
	tickInterval := interval
	if tickInterval > time.Second {
		tickInterval = tickInterval / 2
	}
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.Done():
			return
		case <-ticker.C:
			callTimeout := c.cfg.RequestTimeout
			if callTimeout <= 0 || callTimeout > tickInterval {
				callTimeout = tickInterval
			}
			ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
			request := &keepalivepb.Request{C2S: &keepalivepb.C2S{Time: proto.Int64(time.Now().UTC().Unix())}}
			var response keepalivepb.Response
			err := c.Call(ctx, ProtoKeepAlive, request, &response)
			cancel()
			if err != nil || response.GetRetType() != 0 {
				_ = c.Close()
				return
			}
		}
	}
}

func (c *Client) closeConn(closeNetwork bool) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	conn := c.conn
	c.conn = nil
	pend := c.pend
	c.pend = map[uint32]*pending{}
	c.mu.Unlock()
	for _, p := range pend {
		close(p.ch)
	}
	c.doneOnce.Do(func() { close(c.done) })
	if conn != nil {
		if closeNetwork {
			return conn.Close()
		}
		_ = conn.Close()
	}
	return nil
}

// Subscribe registers a push handler for a given proto ID.
func (c *Client) Subscribe(protoID uint32, fn func(codec.Frame)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subs[protoID] = append(c.subs[protoID], fn)
}

// SubscribeNotify registers a typed handler for OpenD system notifications
// delivered on protocol 1003.
func (c *Client) SubscribeNotify(fn func(*notifypb.Response)) {
	if fn == nil {
		return
	}
	c.Subscribe(ProtoNotify, func(frame codec.Frame) {
		var response notifypb.Response
		if err := proto.Unmarshal(frame.Body, &response); err != nil {
			return
		}
		fn(&response)
	})
}

// Call issues a request and waits for the response on the same serialNo.
func (c *Client) Call(ctx context.Context, protoID uint32, req proto.Message, resp proto.Message) error {
	body, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("opend: marshal request: %w", err)
	}
	serial := atomic.AddUint32(&c.serial, 1)
	pkt, err := codec.Encode(protoID, serial, body)
	if err != nil {
		return err
	}

	p := &pending{ch: make(chan codec.Frame, 1)}
	c.mu.Lock()
	conn := c.conn
	if c.closed || conn == nil {
		c.mu.Unlock()
		return ErrClosed
	}
	c.pend[serial] = p
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pend, serial)
		c.mu.Unlock()
	}()

	c.writeMu.Lock()
	_, err = conn.Write(pkt)
	c.writeMu.Unlock()
	if err != nil {
		return fmt.Errorf("opend: write: %w", err)
	}

	timer := time.NewTimer(c.cfg.RequestTimeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrRequestTimeout
	case f, ok := <-p.ch:
		if !ok {
			return ErrClosed
		}
		if err := proto.Unmarshal(f.Body, resp); err != nil {
			return fmt.Errorf("opend: unmarshal response: %w", err)
		}
		return nil
	}
}

func (c *Client) readLoop(conn net.Conn) {
	head := make([]byte, codec.HeaderLen)
	for {
		if _, err := io.ReadFull(conn, head); err != nil {
			c.fanoutClose()
			return
		}
		bodyLen := int(uint32(head[12]) | uint32(head[13])<<8 | uint32(head[14])<<16 | uint32(head[15])<<24)
		if bodyLen < 0 || bodyLen > codec.MaxFrameBodyLen {
			continue
		}
		data := make([]byte, codec.HeaderLen+bodyLen)
		copy(data, head)
		if _, err := io.ReadFull(conn, data[codec.HeaderLen:]); err != nil {
			c.fanoutClose()
			return
		}
		f, err := codec.Decode(data)
		if err != nil {
			continue
		}
		c.dispatch(f)
	}
}

func (c *Client) dispatch(f codec.Frame) {
	c.mu.Lock()
	p, ok := c.pend[f.Header.SerialNo]
	if ok {
		delete(c.pend, f.Header.SerialNo)
	}
	subs := append([]func(codec.Frame){}, c.subs[f.Header.ProtoID]...)
	c.mu.Unlock()

	if ok {
		select {
		case p.ch <- f:
		default:
		}
		return
	}
	for _, fn := range subs {
		fn(f)
	}
}

func (c *Client) fanoutClose() {
	_ = c.closeConn(true)
}
