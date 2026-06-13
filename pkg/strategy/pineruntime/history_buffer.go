package pineruntime

func newHistoryBuffer(capacity int) *historyBuffer {
	if capacity < 1 {
		capacity = 1
	}
	return &historyBuffer{values: make([]any, capacity)}
}

func (b *historyBuffer) push(value any) {
	if b == nil || len(b.values) == 0 {
		return
	}
	b.values[b.next] = value
	b.next = (b.next + 1) % len(b.values)
	if b.count < len(b.values) {
		b.count++
	}
}

func (b *historyBuffer) lookup(lookback int) (any, bool) {
	if b == nil || lookback <= 0 || lookback > b.count {
		return nil, false
	}
	index := b.next - lookback
	if index < 0 {
		index += len(b.values)
	}
	return b.values[index], true
}
