package pineworker

import "sync"

type TailBuffer struct {
	mu       sync.Mutex
	maxBytes int
	data     []byte
}

func NewTailBuffer(maxBytes int) *TailBuffer {
	if maxBytes <= 0 {
		maxBytes = 4096
	}
	return &TailBuffer{maxBytes: maxBytes}
}

func (buffer *TailBuffer) Write(p []byte) (int, error) {
	if buffer == nil {
		return len(p), nil
	}
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	buffer.data = append(buffer.data, p...)
	if len(buffer.data) > buffer.maxBytes {
		buffer.data = append([]byte(nil), buffer.data[len(buffer.data)-buffer.maxBytes:]...)
	}
	return len(p), nil
}

func (buffer *TailBuffer) String() string {
	if buffer == nil {
		return ""
	}
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return string(buffer.data)
}
