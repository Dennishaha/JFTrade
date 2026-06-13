package indicatorruntime

func (d *monotonicWindowValueDeque) compact() {
	if d == nil || d.start == 0 {
		return
	}
	if d.start >= len(d.values) {
		d.values = d.values[:0]
		d.start = 0
		return
	}
	copy(d.values, d.values[d.start:])
	d.values = d.values[:len(d.values)-d.start]
	d.start = 0
}

func (d *monotonicWindowValueDeque) popExpired(windowStart int) {
	if d == nil {
		return
	}
	for d.start < len(d.values) && d.values[d.start].index < windowStart {
		d.start++
	}
	if d.start > len(d.values)/2 {
		d.compact()
	}
}

func (d *monotonicWindowValueDeque) pushMax(index int, value float64) {
	if d == nil {
		return
	}
	if d.start >= len(d.values) {
		d.values = d.values[:0]
		d.start = 0
	}
	for len(d.values) > d.start && d.values[len(d.values)-1].value <= value {
		d.values = d.values[:len(d.values)-1]
	}
	d.values = append(d.values, windowValue{index: index, value: value})
}

func (d *monotonicWindowValueDeque) pushMin(index int, value float64) {
	if d == nil {
		return
	}
	if d.start >= len(d.values) {
		d.values = d.values[:0]
		d.start = 0
	}
	for len(d.values) > d.start && d.values[len(d.values)-1].value >= value {
		d.values = d.values[:len(d.values)-1]
	}
	d.values = append(d.values, windowValue{index: index, value: value})
}

func (d *monotonicWindowValueDeque) frontValue() (float64, bool) {
	if d == nil || d.start >= len(d.values) {
		return 0, false
	}
	return d.values[d.start].value, true
}

func (w *rollingFloatWindow) ensureCapacity(capacity int) {
	if w == nil || capacity <= 0 {
		return
	}
	if len(w.values) == capacity {
		return
	}
	w.values = make([]float64, capacity)
	w.start = 0
	w.count = 0
}

func (w *rollingFloatWindow) push(value float64, capacity int) (float64, bool) {
	if w == nil || capacity <= 0 {
		return 0, false
	}
	w.ensureCapacity(capacity)
	if w.count < capacity {
		index := (w.start + w.count) % capacity
		w.values[index] = value
		w.count++
		return 0, false
	}
	evicted := w.values[w.start]
	w.values[w.start] = value
	w.start = (w.start + 1) % capacity
	return evicted, true
}

func (w *rollingFloatWindow) len() int {
	if w == nil {
		return 0
	}
	return w.count
}

func (w *rollingFloatWindow) last() (float64, bool) {
	if w == nil || w.count == 0 || len(w.values) == 0 {
		return 0, false
	}
	index := (w.start + w.count - 1) % len(w.values)
	return w.values[index], true
}

func (w *rollingFloatWindow) at(offset int) (float64, bool) {
	if w == nil || offset < 0 || offset >= w.count || len(w.values) == 0 {
		return 0, false
	}
	index := (w.start + offset) % len(w.values)
	return w.values[index], true
}

func (d *indexDeque) reset(capacity int) {
	if capacity <= 0 {
		d.indices = d.indices[:0]
		return
	}
	if cap(d.indices) < capacity {
		d.indices = make([]int, 0, capacity)
		return
	}
	d.indices = d.indices[:0]
}

func (d *indexDeque) popExpired(windowStart int) {
	expired := 0
	for expired < len(d.indices) && d.indices[expired] < windowStart {
		expired++
	}
	if expired == 0 {
		return
	}
	copy(d.indices, d.indices[expired:])
	d.indices = d.indices[:len(d.indices)-expired]
}

func (d *indexDeque) pushMax(values []float64, index int) {
	for len(d.indices) > 0 && values[d.indices[len(d.indices)-1]] <= values[index] {
		d.indices = d.indices[:len(d.indices)-1]
	}
	d.indices = append(d.indices, index)
}

func (d *indexDeque) pushMin(values []float64, index int) {
	for len(d.indices) > 0 && values[d.indices[len(d.indices)-1]] >= values[index] {
		d.indices = d.indices[:len(d.indices)-1]
	}
	d.indices = append(d.indices, index)
}

func (d *indexDeque) front() int {
	if len(d.indices) == 0 {
		return 0
	}
	return d.indices[0]
}
