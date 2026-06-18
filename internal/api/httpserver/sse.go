package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SSEWriter 是线程安全的 SSE 事件写入器。
type SSEWriter struct {
	writer      http.ResponseWriter
	flusher     http.Flusher
	retryMillis int
	mu          *sync.Mutex
}

// SSEStreamLoopOptions 控制 SSE 事件循环的行为。
type SSEStreamLoopOptions struct {
	// Initial 在循环开始前调用一次（可选）。
	Initial func(context.Context) error
	// WriteInterval 周期性 tick 间隔。为 0 时不启用周期 tick。
	WriteInterval time.Duration
	// OnTick 在每次 tick 或 trigger 时调用。
	OnTick func(context.Context) error
	// Trigger 可选的触发通道。收到事件时立即调用 OnTick。
	Trigger <-chan struct{}
}

// WriteSSEHeaders 写入标准 SSE HTTP 响应头。
func WriteSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

// NewSSEWriter 创建一个 SSEWriter。
func NewSSEWriter(w http.ResponseWriter, flusher http.Flusher, retryMillis int) SSEWriter {
	return SSEWriter{
		writer:      w,
		flusher:     flusher,
		retryMillis: retryMillis,
		mu:          &sync.Mutex{},
	}
}

// PrepareSSEWriter 准备 ResponseWriter 用于 SSE 写入。返回 SSEWriter 和是否成功。
func PrepareSSEWriter(w http.ResponseWriter) (SSEWriter, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return SSEWriter{}, false
	}
	WriteSSEHeaders(w)
	return NewSSEWriter(w, flusher, 3000), true
}

// WriteRetryDirective 写入 SSE retry 指令（毫秒）。
func (w SSEWriter) WriteRetryDirective() error {
	if w.retryMillis <= 0 {
		return nil
	}
	return w.writeFrame(fmt.Sprintf("retry: %d\n\n", w.retryMillis))
}

func (w SSEWriter) writeFrame(frame string) (err error) {
	if w.mu != nil {
		w.mu.Lock()
		defer w.mu.Unlock()
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("sse write failed: %v", recovered)
		}
	}()
	if _, err := fmt.Fprint(w.writer, frame); err != nil {
		return err
	}
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return nil
}

// WriteEvent 将 value JSON 序列化后作为 SSE data 事件写入。
func (w SSEWriter) WriteEvent(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return w.writeFrame(fmt.Sprintf("data: %s\n\n", data))
}

// WriteEventID writes a JSON SSE event with a resumable event identifier.
func (w SSEWriter) WriteEventID(id string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return w.writeFrame(fmt.Sprintf("id: %s\ndata: %s\n\n", id, data))
}

// WriteComment keeps an otherwise idle SSE connection alive.
func (w SSEWriter) WriteComment(comment string) error {
	return w.writeFrame(fmt.Sprintf(": %s\n\n", comment))
}

// RunSSEStreamLoop 运行 SSE 事件循环，在 ctx 取消时返回。
func RunSSEStreamLoop(ctx context.Context, opts SSEStreamLoopOptions) error {
	if opts.Initial != nil {
		if err := opts.Initial(ctx); err != nil {
			return err
		}
	}

	var ticker *time.Ticker
	if opts.WriteInterval > 0 && opts.OnTick != nil {
		ticker = time.NewTicker(opts.WriteInterval)
		defer ticker.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-opts.Trigger:
			if opts.OnTick == nil {
				continue
			}
			if err := opts.OnTick(ctx); err != nil {
				return err
			}
		case <-tickerC(ticker):
			if opts.OnTick == nil {
				continue
			}
			if err := opts.OnTick(ctx); err != nil {
				return err
			}
		}
	}
}

func tickerC(ticker *time.Ticker) <-chan time.Time {
	if ticker == nil {
		return nil
	}
	return ticker.C
}
