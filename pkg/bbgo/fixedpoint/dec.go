//go:build dnum

package fixedpoint

import (
	"math"
	"math/bits"
)

type Value struct {
	coef uint64
	sign int
	exp  int
}

const (
	signPosInf = +2
	signPos    = +1
	signZero   = 0
	signNeg    = -1
	signNegInf = -2
	coefMin    = 1000_0000_0000_0000
	coefMax    = 9999_9999_9999_9999
	digitsMax  = 16
	shiftMax   = digitsMax - 1
	// to switch between scientific notion and normal presentation format
	maxLeadingZeros = 19
)

// common values
var (
	Zero   = Value{}
	One    = Value{1000_0000_0000_0000, signPos, 1}
	NegOne = Value{1000_0000_0000_0000, signNeg, 1}
	PosInf = Value{1, signPosInf, 0}
	NegInf = Value{1, signNegInf, 0}
)

var pow10f = [...]float64{
	1,
	10,
	100,
	1000,
	10000,
	100000,
	1000000,
	10000000,
	100000000,
	1000000000,
	10000000000,
	100000000000,
	1000000000000,
	10000000000000,
	100000000000000,
	1000000000000000,
	10000000000000000,
	100000000000000000,
	1000000000000000000,
	10000000000000000000,
	100000000000000000000}

var pow10 = [...]uint64{
	1,
	10,
	100,
	1000,
	10000,
	100000,
	1000000,
	10000000,
	100000000,
	1000000000,
	10000000000,
	100000000000,
	1000000000000,
	10000000000000,
	100000000000000,
	1000000000000000,
	10000000000000000,
	100000000000000000,
	1000000000000000000}

var halfpow10 = [...]uint64{
	0,
	5,
	50,
	500,
	5000,
	50000,
	500000,
	5000000,
	50000000,
	500000000,
	5000000000,
	50000000000,
	500000000000,
	5000000000000,
	50000000000000,
	500000000000000,
	5000000000000000,
	50000000000000000,
	500000000000000000,
	5000000000000000000}

// NewFromInt returns a Value for an int
func NewFromInt(n int64) Value {
	if n == 0 {
		return Zero
	}
	// n0 := n
	sign := signPos
	if n < 0 {
		n = -n
		sign = signNeg
	}
	return newNoSignCheck(sign, uint64(n), digitsMax)
}

const log2of10 = 3.32192809488736234

// NewFromFloat converts a float64 to a Value
func NewFromFloat(f float64) Value {
	switch {
	case math.IsInf(f, +1):
		return PosInf
	case math.IsInf(f, -1):
		return NegInf
	case math.IsNaN(f):
		panic("value.NewFromFloat can't convert NaN")
	}

	if f == 0 {
		return Zero
	}

	sign := signPos
	if f < 0 {
		f = -f
		sign = signNeg
	}
	n := uint64(f)
	if float64(n) == f {
		return newNoSignCheck(sign, n, digitsMax)
	}
	_, e := math.Frexp(f)
	e = int(float32(e)/log2of10) - 16
	var c uint64
	if e < 0 && e > -lenPow10 {
		c = uint64(f*pow10f[-e] + 0.5)
	} else if e >= 0 && e < lenPow10 {
		c = uint64(f/pow10f[e] + 0.5)
	} else {
		c = uint64(f/math.Pow10(e) + 0.5)
	}
	return newNoSignCheck(sign, c, e+16)
}

// Raw constructs a Value without normalizing - arguments must be valid.
// Used by SuValue Unpack
func Raw(sign int, coef uint64, exp int) Value {
	return Value{coef, sign, int(exp)}
}

func newNoSignCheck(sign int, coef uint64, exp int) Value {
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
	return Value{coef, sign, exp}
}

// New constructs a Value, maximizing coef and handling exp out of range
// Used to normalize results of operations
func New(sign int, coef uint64, exp int) Value {
	if sign == 0 || coef == 0 {
		return Zero
	} else if sign == signPosInf {
		return PosInf
	} else if sign == signNegInf {
		return NegInf
	} else {
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
		return Value{coef, sign, exp}
	}
}

func maxShift(x uint64) int {
	i := ilog10(x)
	if i > shiftMax {
		return 0
	}
	return shiftMax - i
}

func ilog10(x uint64) int {
	// based on Hacker's Delight
	if x == 0 {
		return 0
	}
	y := (19 * (63 - bits.LeadingZeros64(x))) >> 6
	if y < 18 && x >= pow10[y+1] {
		y++
	}
	return y
}

func Inf(sign int) Value {
	switch {
	case sign < 0:
		return NegInf
	case sign > 0:
		return PosInf
	default:
		return Zero
	}
}

// IsInf returns true if a Value is positive or negative infinite
func (dn Value) IsInf() bool {
	return dn.sign == signPosInf || dn.sign == signNegInf
}

// IsZero returns true if a Value is zero
func (dn Value) IsZero() bool {
	return dn.sign == signZero
}

const lenPow10 = len(pow10)

// Float64 converts a Value to float64
func (dn Value) Float64() float64 {
	if dn.IsInf() {
		return math.Inf(int(dn.sign))
	}
	g := float64(dn.coef)
	if dn.sign == signNeg {
		g = -g
	}
	i := int(dn.exp) - digitsMax

	if i < 0 && i > -lenPow10 {
		return g / pow10f[-i]
	} else if i >= 0 && i < lenPow10 {
		return g * pow10f[i]
	}
	return g * math.Pow(10, float64(i))
}

// Int64 converts a Value to an int64, returning whether it was convertible
func (dn Value) Int64() int64 {
	if dn.sign == 0 {
		return 0
	}
	if dn.sign != signNegInf && dn.sign != signPosInf {
		digitDiff := digitsMax - dn.exp
		if 0 < dn.exp && digitDiff > 0 {
			return int64(dn.sign) * int64(dn.coef/pow10[digitDiff])
		} else if dn.exp <= 0 && dn.coef != 0 {
			result := math.Log10(float64(dn.coef)) - float64(digitsMax) + float64(dn.exp)
			return int64(dn.sign) * int64(math.Pow(10, result))
		}
		switch digitDiff {
		case 0:
			return int64(dn.sign) * int64(dn.coef)
		case 1:
			return int64(dn.sign) * (int64(dn.coef) * 10)
		case 2:
			return int64(dn.sign) * (int64(dn.coef) * 100)
		case 3:
			if dn.coef < math.MaxInt64/1000 {
				return int64(dn.sign) * (int64(dn.coef) * 1000)
			}
		}
	}
	panic("unable to convert Value to int64")
}

func (dn Value) Int() int {
	// if int is int64, this is a nop
	n := dn.Int64()
	if int64(int(n)) != n {
		panic("unable to convert Value to int32")
	}
	return int(n)
}

// Sign returns -1 for negative, 0 for zero, and +1 for positive
func (dn Value) Sign() int {
	return dn.sign
}

// Coef returns the coefficient
func (dn Value) Coef() uint64 {
	return dn.coef
}

// Exp returns the exponent
func (dn Value) Exp() int {
	return dn.exp
}

// Frac returns the fractional portion, i.e. x - x.Int()
func (dn Value) Frac() Value {
	if dn.sign == 0 || dn.sign == signNegInf || dn.sign == signPosInf ||
		dn.exp >= digitsMax {
		return Zero
	}
	if dn.exp <= 0 {
		return dn
	}
	frac := dn.coef % pow10[digitsMax-dn.exp]
	if frac == dn.coef {
		return dn
	}
	return New(dn.sign, frac, int(dn.exp))
}

type RoundingMode int

const (
	Up RoundingMode = iota
	Down
	HalfUp
)

// Trunc returns the integer portion (truncating any fractional part)
func (dn Value) Trunc() Value {
	return dn.integer(Down)
}

func (dn Value) integer(mode RoundingMode) Value {
	if dn.sign == 0 || dn.sign == signNegInf || dn.sign == signPosInf ||
		dn.exp >= digitsMax {
		return dn
	}
	if dn.exp <= 0 {
		if mode == Up ||
			(mode == HalfUp && dn.exp == 0 && dn.coef >= One.coef*5) {
			return New(dn.sign, One.coef, int(dn.exp)+1)
		}
		return Zero
	}
	e := digitsMax - dn.exp
	frac := dn.coef % pow10[e]
	if frac == 0 {
		return dn
	}
	i := dn.coef - frac
	if (mode == Up && frac > 0) || (mode == HalfUp && frac >= halfpow10[e]) {
		return New(dn.sign, i+pow10[e], int(dn.exp)) // normalize
	}
	return Value{i, dn.sign, dn.exp}
}

func (dn Value) Floor() Value {
	return dn.Round(0, Down)
}

func (dn Value) Round(r int, mode RoundingMode) Value {
	if dn.sign == 0 || dn.sign == signNegInf || dn.sign == signPosInf ||
		r >= digitsMax {
		return dn
	}
	if r <= -digitsMax {
		return Zero
	}
	n := New(dn.sign, dn.coef, int(dn.exp)+r) // multiply by 10^r
	n = n.integer(mode)
	if n.sign == signPos || n.sign == signNeg { // i.e. not zero or inf
		return New(n.sign, n.coef, int(n.exp)-r)
	}
	return n
}

// arithmetic operations -------------------------------------------------------

// Neg returns the Value negated i.e. sign reversed
func (dn Value) Neg() Value {
	return Value{dn.coef, -dn.sign, dn.exp}
}

// Abs returns the Value with a positive sign
func (dn Value) Abs() Value {
	if dn.sign < 0 {
		return Value{dn.coef, -dn.sign, dn.exp}
	}
	return dn
}

// Equal returns true if two Value's are equal
func Equal(x, y Value) bool {
	return x.sign == y.sign && x.exp == y.exp && x.coef == y.coef
}

func (x Value) Eq(y Value) bool {
	return Equal(x, y)
}

func Max(x, y Value) Value {
	if Compare(x, y) > 0 {
		return x
	}
	return y
}

func Min(x, y Value) Value {
	if Compare(x, y) < 0 {
		return x
	}
	return y
}

// Compare compares two Value's returning -1 for <, 0 for ==, +1 for >
func Compare(x, y Value) int {
	switch {
	case x.sign < y.sign:
		return -1
	case x.sign > y.sign:
		return 1
	case x == y:
		return 0
	}
	sign := int(x.sign)
	switch {
	case sign == 0 || sign == signNegInf || sign == signPosInf:
		return 0
	case x.exp < y.exp:
		return -sign
	case x.exp > y.exp:
		return +sign
	case x.coef < y.coef:
		return -sign
	case x.coef > y.coef:
		return +sign
	default:
		return 0
	}
}

func (x Value) Compare(y Value) int {
	return Compare(x, y)
}

func Must(v Value, err error) Value {
	if err != nil {
		panic(err)
	}
	return v
}

// v * 10^(exp)
func (v Value) MulExp(exp int) Value {
	return Value{v.coef, v.sign, v.exp + exp}
}

// Sub returns the difference of two Value's
func Sub(x, y Value) Value {
	return Add(x, y.Neg())
}

func (x Value) Sub(y Value) Value {
	return Sub(x, y)
}

// Add returns the sum of two Value's
func Add(x, y Value) Value {
	switch {
	case x.sign == signZero:
		return y
	case y.sign == signZero:
		return x
	case x.IsInf():
		if y.sign == -x.sign {
			return Zero
		}
		return x
	case y.IsInf():
		return y
	}
	if !align(&x, &y) {
		return x
	}
	if x.sign != y.sign {
		return usub(x, y)
	}
	return uadd(x, y)
}

func (x Value) Add(y Value) Value {
	return Add(x, y)
}

func uadd(x, y Value) Value {
	return New(x.sign, x.coef+y.coef, int(x.exp))
}

func usub(x, y Value) Value {
	if x.coef < y.coef {
		return New(-x.sign, y.coef-x.coef, int(x.exp))
	}
	return New(x.sign, x.coef-y.coef, int(x.exp))
}

func align(x, y *Value) bool {
	if x.exp == y.exp {
		return true
	}
	if x.exp < y.exp {
		*x, *y = *y, *x // swap
	}
	yshift := ilog10(y.coef)
	e := int(x.exp - y.exp)
	if e > yshift {
		return false
	}
	yshift = e
	// check(0 <= yshift && yshift <= 20)
	// y.coef = (y.coef + halfpow10[yshift]) / pow10[yshift]
	y.coef = (y.coef) / pow10[yshift]
	// check(int(y.exp)+yshift == int(x.exp))
	return true
}

// Mul returns the product of two Value's
func Mul(x, y Value) Value {
	sign := x.sign * y.sign
	switch {
	case sign == signZero:
		return Zero
	case x.IsInf() || y.IsInf():
		return Inf(sign)
	}
	e := int(x.exp) + int(y.exp)

	// split unevenly to use full 64 bit range to get more precision
	// and avoid needing xlo * ylo
	xhi := x.coef / 1e7 // 9 digits
	xlo := x.coef % 1e7 // 7 digits
	yhi := y.coef / 1e7 // 9 digits
	ylo := y.coef % 1e7 // 7 digits

	c := xhi * yhi
	if (xlo | ylo) != 0 {
		c += (xlo*yhi + ylo*xhi) / 1e7
	}
	return New(sign, c, e-2)
}

func (x Value) Mul(y Value) Value {
	return Mul(x, y)
}

// Div returns the quotient of two Value's
func Div(x, y Value) Value {
	sign := x.sign * y.sign
	switch {
	case x.sign == signZero:
		return x
	case y.sign == signZero:
		return Inf(x.sign)
	case x.IsInf():
		if y.IsInf() {
			if sign < 0 {
				return NegOne
			}
			return One
		}
		return Inf(sign)
	case y.IsInf():
		return Zero
	}
	coef := div128(x.coef, y.coef)
	return New(sign, coef, int(x.exp)-int(y.exp))
}

func (x Value) Div(y Value) Value {
	return Div(x, y)
}

// Hash returns a hash value for a Value
func (dn Value) Hash() uint32 {
	return uint32(dn.coef>>32) ^ uint32(dn.coef) ^
		uint32(dn.sign)<<16 ^ uint32(dn.exp)<<8
}

func Clamp(x, min, max Value) Value {
	if x.Compare(min) < 0 {
		return min
	}
	if x.Compare(max) > 0 {
		return max
	}
	return x
}

func (x Value) Clamp(min, max Value) Value {
	if x.Compare(min) < 0 {
		return min
	}
	if x.Compare(max) > 0 {
		return max
	}
	return x
}
