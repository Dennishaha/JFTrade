//go:build dnum

package fixedpoint

import (
	"bytes"
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"strings"
)

func (v Value) Value() (driver.Value, error) {
	return v.Float64(), nil
}

func (v *Value) Scan(src interface{}) error {
	var err error
	switch d := src.(type) {
	case int64:
		*v = NewFromInt(d)
		return nil
	case float64:
		*v = NewFromFloat(d)
		return nil
	case []byte:
		*v, err = NewFromString(string(d))
		if err != nil {
			return err
		}
		return nil
	default:
	}
	return fmt.Errorf("fixedpoint.Value scan error, type %T is not supported, value: %+v", src, src)
}

// NewFromString parses a numeric string and returns a Value representation.
func NewFromString(s string) (Value, error) {
	length := len(s)
	if length == 0 {
		return Zero, nil
	}
	isPercentage := s[length-1] == '%'
	if isPercentage {
		s = s[:length-1]
	}
	r := &reader{s, 0}
	sign := r.getSign()
	if r.matchStrIgnoreCase("inf") {
		return Inf(sign), nil
	}
	coef, exp := r.getCoef()
	exp += r.getExp()
	if r.len() != 0 {
		return Zero, errors.New("invalid number")
	} else if coef == 0 || exp < math.MinInt8 {
		return Zero, nil
	} else if exp > math.MaxInt8 {
		return Inf(sign), nil
	}
	if isPercentage {
		exp -= 2
	}
	atmax := false
	for coef > coefMax {
		coef = (coef + 5) / 10
		exp++
		atmax = true
	}

	if !atmax {
		p := maxShift(coef)
		coef *= pow10[p]
		exp -= p
	}
	return Value{coef, sign, exp}, nil
}

func MustNewFromString(input string) Value {
	v, err := NewFromString(input)
	if err != nil {
		panic(fmt.Errorf("cannot parse %s into fixedpoint, error: %s", input, err.Error()))
	}
	return v
}

func NewFromBytes(s []byte) (Value, error) {
	length := len(s)
	if length == 0 {
		return Zero, nil
	}
	isPercentage := s[length-1] == '%'
	if isPercentage {
		s = s[:length-1]
	}
	r := &readerBytes{s, 0}
	sign := r.getSign()
	if r.matchStrIgnoreCase("inf") {
		return Inf(sign), nil
	}
	coef, exp := r.getCoef()
	exp += r.getExp()
	if r.len() != 0 {
		return Zero, errors.New("invalid number")
	} else if coef == 0 || exp < math.MinInt8 {
		return Zero, nil
	} else if exp > math.MaxInt8 {
		return Inf(sign), nil
	}
	if isPercentage {
		exp -= 2
	}
	atmax := false
	for coef > coefMax {
		coef = (coef + 5) / 10
		exp++
		atmax = true
	}

	if !atmax {
		p := maxShift(coef)
		coef *= pow10[p]
		exp -= p
	}
	return Value{coef, sign, exp}, nil
}

func MustNewFromBytes(input []byte) Value {
	v, err := NewFromBytes(input)
	if err != nil {
		panic(fmt.Errorf("cannot parse %s into fixedpoint, error: %s", input, err.Error()))
	}
	return v
}

type readerBytes struct {
	s []byte
	i int
}

func (r *readerBytes) cur() byte {
	if r.i >= len(r.s) {
		return 0
	}
	return byte(r.s[r.i])
}

func (r *readerBytes) prev() byte {
	if r.i == 0 {
		return 0
	}
	return byte(r.s[r.i-1])
}

func (r *readerBytes) len() int {
	return len(r.s) - r.i
}

func (r *readerBytes) match(c byte) bool {
	if r.cur() == c {
		r.i++
		return true
	}
	return false
}

func (r *readerBytes) matchDigit() bool {
	c := r.cur()
	if '0' <= c && c <= '9' {
		r.i++
		return true
	}
	return false
}

func (r *readerBytes) matchStrIgnoreCase(pre string) bool {
	pre = strings.ToLower(pre)
	boundary := r.i + len(pre)
	if boundary > len(r.s) {
		return false
	}
	for i, c := range bytes.ToLower(r.s[r.i:boundary]) {
		if pre[i] != c {
			return false
		}
	}
	r.i = boundary
	return true
}

func (r *readerBytes) getSign() int {
	if r.match('-') {
		return signNeg
	}
	r.match('+')
	return signPos
}

func (r *readerBytes) getCoef() (uint64, int) {
	digits := false
	beforeDecimal := true
	for r.match('0') {
		digits = true
	}
	if r.cur() == '.' && r.len() > 1 {
		digits = false
	}
	n := uint64(0)
	exp := 0
	p := shiftMax
	for {
		c := r.cur()
		if r.matchDigit() {
			digits = true
			if c != '0' && p >= 0 {
				n += uint64(c-'0') * pow10[p]
			}
			p--
		} else if beforeDecimal {
			exp = shiftMax - p
			if !r.match('.') {
				break
			}
			beforeDecimal = false
			if !digits {
				for r.match('0') {
					digits = true
					exp--
				}
			}
		} else {
			break
		}
	}
	if !digits {
		panic("numbers require at least one digit")
	}
	return n, exp
}

func (r *readerBytes) getExp() int {
	e := 0
	if r.match('e') || r.match('E') {
		esign := r.getSign()
		for r.matchDigit() {
			e = e*10 + int(r.prev()-'0')
		}
		e *= int(esign)
	}
	return e
}

type reader struct {
	s string
	i int
}

func (r *reader) cur() byte {
	if r.i >= len(r.s) {
		return 0
	}
	return byte(r.s[r.i])
}

func (r *reader) prev() byte {
	if r.i == 0 {
		return 0
	}
	return byte(r.s[r.i-1])
}

func (r *reader) len() int {
	return len(r.s) - r.i
}

func (r *reader) match(c byte) bool {
	if r.cur() == c {
		r.i++
		return true
	}
	return false
}

func (r *reader) matchDigit() bool {
	c := r.cur()
	if '0' <= c && c <= '9' {
		r.i++
		return true
	}
	return false
}

func (r *reader) matchStrIgnoreCase(pre string) bool {
	boundary := r.i + len(pre)
	if boundary > len(r.s) {
		return false
	}
	data := strings.ToLower(r.s[r.i:boundary])
	pre = strings.ToLower(pre)
	if data == pre {
		r.i = boundary
		return true
	}
	return false
}

func (r *reader) getSign() int {
	if r.match('-') {
		return signNeg
	}
	r.match('+')
	return signPos
}

func (r *reader) getCoef() (uint64, int) {
	digits := false
	beforeDecimal := true
	for r.match('0') {
		digits = true
	}
	if r.cur() == '.' && r.len() > 1 {
		digits = false
	}
	n := uint64(0)
	exp := 0
	p := shiftMax
	for {
		c := r.cur()
		if r.matchDigit() {
			digits = true
			if c != '0' && p >= 0 {
				n += uint64(c-'0') * pow10[p]
			}
			p--
		} else if beforeDecimal {
			exp = shiftMax - p
			if !r.match('.') {
				break
			}
			beforeDecimal = false
			if !digits {
				for r.match('0') {
					digits = true
					exp--
				}
			}
		} else {
			break
		}
	}
	if !digits {
		panic("numbers require at least one digit")
	}
	return n, exp
}

func (r *reader) getExp() int {
	e := 0
	if r.match('e') || r.match('E') {
		esign := r.getSign()
		for r.matchDigit() {
			e = e*10 + int(r.prev()-'0')
		}
		e *= int(esign)
	}
	return e
}

func (v Value) MarshalYAML() (interface{}, error) {
	return v.FormatString(8), nil
}

func (v *Value) UnmarshalYAML(unmarshal func(a interface{}) error) (err error) {
	var f float64
	if err = unmarshal(&f); err == nil {
		*v = NewFromFloat(f)
		return
	}
	var i int64
	if err = unmarshal(&i); err == nil {
		*v = NewFromInt(i)
		return
	}

	var s string
	if err = unmarshal(&s); err == nil {
		nv, err2 := NewFromString(s)
		if err2 == nil {
			*v = nv
			return
		}
	}
	return err
}

// FIXME: should we limit to 8 prec?
func (v Value) MarshalJSON() ([]byte, error) {
	if v.IsInf() {
		return []byte("\"" + v.String() + "\""), nil
	}
	return []byte(v.FormatString(8)), nil
}

func (v *Value) UnmarshalJSON(data []byte) error {
	if bytes.Compare(data, []byte{'n', 'u', 'l', 'l'}) == 0 {
		*v = Zero
		return nil
	}
	if len(data) == 0 {
		*v = Zero
		return nil
	}
	var err error
	if data[0] == '"' {
		data = data[1 : len(data)-1]
	}
	if *v, err = NewFromBytes(data); err != nil {
		return err
	}
	return nil
}
