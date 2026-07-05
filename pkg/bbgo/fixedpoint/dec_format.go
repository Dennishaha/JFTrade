//go:build dnum

package fixedpoint

import (
	"bytes"
	"strconv"
	"strings"
)

func (dn Value) FormatString(prec int) string {
	if dn.sign == 0 {
		if prec <= 0 {
			return "0"
		} else {
			return "0." + strings.Repeat("0", prec)
		}
	}
	sign := ""
	if dn.sign < 0 {
		sign = "-"
	}
	if dn.IsInf() {
		return sign + "inf"
	}
	digits := getDigits(dn.coef)
	nd := len(digits)
	e := int(dn.exp) - nd
	if -maxLeadingZeros <= dn.exp && dn.exp <= 0 {
		if prec < 0 {
			return "0"
		}
		if prec+e+nd > 0 {
			return sign + "0." + strings.Repeat("0", -e-nd) + digits[:min(prec+e+nd, nd)] + strings.Repeat("0", max(0, prec-nd+e+nd))
		} else if -e-nd > 0 && prec != 0 {
			return "0." + strings.Repeat("0", min(prec, -e-nd))
		} else {
			return "0"
		}
	} else if -nd < e && e <= -1 {
		dec := nd + e
		if prec > 0 {
			decimals := digits[dec:min(dec+prec, nd)]
			return sign + digits[:dec] + "." + decimals + strings.Repeat("0", max(0, prec-len(decimals)))
		} else if prec == 0 {
			return sign + digits[:dec]
		}

		sigFigures := digits[0:max(dec+prec, 0)]
		if len(sigFigures) == 0 {
			return "0"
		}

		return sign + sigFigures + strings.Repeat("0", max(-prec, 0))

	} else if 0 < dn.exp && dn.exp <= digitsMax {
		if prec > 0 {
			return sign + digits + strings.Repeat("0", e) + "." + strings.Repeat("0", prec)
		} else if prec+e >= 0 {
			return sign + digits + strings.Repeat("0", e)
		} else {
			if len(digits) <= -prec-e {
				return "0"
			}

			return sign + digits[0:len(digits)+prec+e] + strings.Repeat("0", -prec)
		}
	} else {
		after := ""
		if nd > 1 {
			after = "." + digits[1:min(1+prec, nd)] + strings.Repeat("0", max(0, min(1+prec, nd)-1-prec))
		}
		return sign + digits[:1] + after + "e" + strconv.Itoa(int(dn.exp-1))
	}
}

// String returns a string representation of the Value
func (dn Value) String() string {
	if dn.sign == 0 {
		return "0"
	}
	sign := ""
	if dn.sign < 0 {
		sign = "-"
	}
	if dn.IsInf() {
		return sign + "inf"
	}
	digits := getDigits(dn.coef)
	nd := len(digits)
	e := int(dn.exp) - nd
	if -maxLeadingZeros <= dn.exp && dn.exp <= 0 {
		return sign + "0." + strings.Repeat("0", -e-nd) + digits
	} else if -nd < e && e <= -1 {
		dec := nd + e
		return sign + digits[:dec] + "." + digits[dec:]
	} else if 0 < dn.exp && dn.exp <= digitsMax {
		return sign + digits + strings.Repeat("0", e)
	} else {
		after := ""
		if nd > 1 {
			after = "." + digits[1:]
		}
		return sign + digits[:1] + after + "e" + strconv.Itoa(int(dn.exp-1))
	}
}

func (dn Value) Percentage() string {
	if dn.sign == 0 {
		return "0%"
	}
	sign := ""
	if dn.sign < 0 {
		sign = "-"
	}
	if dn.IsInf() {
		return sign + "inf%"
	}
	digits := getDigits(dn.coef)
	nd := len(digits)
	e := int(dn.exp) - nd + 2

	if -maxLeadingZeros <= dn.exp && dn.exp <= -2 {
		return sign + "0." + strings.Repeat("0", -e-nd) + digits + "%"
	} else if -nd < e && e <= -1 {
		dec := nd + e
		return sign + digits[:dec] + "." + digits[dec:] + "%"
	} else if -2 < dn.exp && dn.exp <= digitsMax {
		return sign + digits + strings.Repeat("0", e) + "%"
	} else {
		after := ""
		if nd > 1 {
			after = "." + digits[1:]
		}
		return sign + digits[:1] + after + "e" + strconv.Itoa(int(dn.exp-1)) + "%"
	}
}

func (dn Value) FormatPercentage(prec int) string {
	if dn.sign == 0 {
		if prec <= 0 {
			return "0"
		} else {
			return "0." + strings.Repeat("0", prec)
		}
	}
	sign := ""
	if dn.sign < 0 {
		sign = "-"
	}
	if dn.IsInf() {
		return sign + "inf"
	}
	digits := getDigits(dn.coef)
	nd := len(digits)
	exp := dn.exp + 2
	e := int(exp) - nd

	if -maxLeadingZeros <= exp && exp <= 0 {
		if prec+e+nd > 0 {
			return sign + "0." + strings.Repeat("0", -e-nd) + digits[:min(prec+e+nd, nd)] + strings.Repeat("0", max(0, prec-nd+e+nd)) + "%"
		} else if -e-nd > 0 {
			return "0." + strings.Repeat("0", -e-nd) + "%"
		} else {
			return "0"
		}
	} else if -nd < e && e <= -1 {
		dec := nd + e
		decimals := digits[dec:min(dec+prec, nd)]
		return sign + digits[:dec] + "." + decimals + strings.Repeat("0", max(0, prec-len(decimals))) + "%"
	} else if 0 < exp && exp <= digitsMax {
		if prec > 0 {
			return sign + digits + strings.Repeat("0", e) + "." + strings.Repeat("0", prec) + "%"
		} else {
			return sign + digits + strings.Repeat("0", e) + "%"
		}
	} else {
		after := ""
		if nd > 1 {
			after = "." + digits[1:min(1+prec, nd)] + strings.Repeat("0", max(0, min(1+prec, nd)-1-prec))
		}
		return sign + digits[:1] + after + "e" + strconv.Itoa(int(exp-1)) + "%"
	}
}

func (dn Value) SignedPercentage() string {
	if dn.Sign() >= 0 {
		return "+" + dn.Percentage()
	}
	return dn.Percentage()
}

// get digit length
func (a Value) NumDigits() int {
	i := shiftMax
	coef := a.coef
	nd := 0
	for coef != 0 && coef < pow10[i] {
		i--
	}
	for coef != 0 {
		coef %= pow10[i]
		i--
		nd++
	}
	return nd
}

// alias of Exp
func (a Value) NumIntDigits() int {
	return a.exp
}

// get fractional digits
func (a Value) NumFractionalDigits() int {
	nd := a.NumDigits()
	return nd - a.exp
}

func getDigits(coef uint64) string {
	var digits [digitsMax]byte
	i := shiftMax
	nd := 0
	for coef != 0 {
		digits[nd] = byte('0' + (coef / pow10[i]))
		coef %= pow10[i]
		nd++
		i--
	}
	return string(digits[:nd])
}

// Format converts a number to a string with a specified format
func (dn Value) Format(mask string) string {
	if dn.IsInf() {
		return "#"
	}
	n := dn
	before := 0
	after := 0
	intpart := true
	for _, mc := range mask {
		switch mc {
		case '.':
			intpart = false
		case '#':
			if intpart {
				before++
			} else {
				after++
			}
		}
	}
	if before+after == 0 || n.Exp() > before {
		return "#"
	}
	n = n.Round(after, HalfUp)
	e := n.Exp()
	var digits []byte
	if n.IsZero() && after == 0 {
		digits = []byte("0")
		e = 1
	} else {
		digits = strconv.AppendUint(make([]byte, 0, digitsMax), n.Coef(), 10)
		digits = bytes.TrimRight(digits, "0")
	}
	nd := len(digits)

	di := e - before
	var buf strings.Builder
	sign := n.Sign()
	signok := sign >= 0
	frac := false
	for _, mc := range []byte(mask) {
		switch mc {
		case '#':
			if 0 <= di && di < nd {
				buf.WriteByte(digits[di])
			} else if frac || di >= 0 {
				buf.WriteByte('0')
			}
			di++
		case ',':
			if di > 0 {
				buf.WriteByte(',')
			}
		case '-', '(':
			signok = true
			if sign < 0 {
				buf.WriteByte(mc)
			}
		case ')':
			if sign < 0 {
				buf.WriteByte(mc)
			} else {
				buf.WriteByte(' ')
			}
		case '.':
			frac = true
			fallthrough
		default:
			buf.WriteByte(mc)
		}
	}
	if !signok {
		return "-"
	}
	return buf.String()
}
