use std::fmt;
use std::str::FromStr;

use rust_decimal::Decimal;
use rust_decimal::prelude::{FromPrimitive, ToPrimitive};
use serde::{Deserialize, Deserializer, Serialize, Serializer};

use crate::CodecError;
use crate::decimal::{ParsedDecimal, parse_decimal};

const SCALE: u64 = 100_000_000;

#[derive(Clone, Copy, Debug, Default, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct Fixed8 {
    non_finite: i8, // -1: NEG_INFINITY, 0: Finite, 1: POS_INFINITY
    scaled: i64,
}

impl Fixed8 {
    pub const NEG_INFINITY: Self = Self {
        non_finite: -1,
        scaled: i64::MIN,
    };
    pub const POS_INFINITY: Self = Self {
        non_finite: 1,
        scaled: i64::MAX,
    };
    pub const ZERO: Self = Self {
        non_finite: 0,
        scaled: 0,
    };

    pub const fn from_scaled(scaled: i64) -> Self {
        Self {
            non_finite: 0,
            scaled,
        }
    }

    pub const fn scaled(self) -> i64 {
        self.scaled
    }

    pub const fn is_zero(self) -> bool {
        self.non_finite == 0 && self.scaled == 0
    }

    pub const fn is_finite(self) -> bool {
        self.non_finite == 0
    }

    pub const fn signum(self) -> i8 {
        if self.non_finite != 0 {
            self.non_finite
        } else if self.scaled > 0 {
            1
        } else if self.scaled < 0 {
            -1
        } else {
            0
        }
    }

    pub fn to_decimal(self) -> Decimal {
        if self.non_finite == 1 {
            Decimal::MAX
        } else if self.non_finite == -1 {
            Decimal::MIN
        } else {
            Decimal::from_i128_with_scale(self.scaled as i128, 8)
        }
    }

    pub fn to_decimal_checked(self) -> Result<Decimal, CodecError> {
        self.ensure_finite()?;
        Ok(Decimal::from_i128_with_scale(self.scaled as i128, 8))
    }

    pub fn from_decimal(decimal: Decimal) -> Result<Self, CodecError> {
        if decimal == Decimal::MAX {
            return Ok(Self::POS_INFINITY);
        }
        if decimal == Decimal::MIN {
            return Ok(Self::NEG_INFINITY);
        }
        let scaled = decimal
            .checked_mul(Decimal::from(SCALE))
            .ok_or(CodecError::Fixed8ArithmeticOverflow)?
            .trunc();
        let mantissa = scaled.mantissa();
        let val = i64::try_from(mantissa).map_err(|_| CodecError::Fixed8OutOfRange)?;
        Ok(Self::from_scaled(val))
    }

    pub fn to_f64(self) -> Result<f64, CodecError> {
        self.ensure_finite()?;
        let dec = self.to_decimal_checked()?;
        dec.to_f64().ok_or(CodecError::Fixed8ArithmeticOverflow)
    }

    pub fn from_f64(value: f64) -> Result<Self, CodecError> {
        if !value.is_finite() {
            return Err(CodecError::Fixed8ArithmeticOverflow);
        }
        let dec = Decimal::from_f64(value).ok_or(CodecError::Fixed8ArithmeticOverflow)?;
        Self::from_decimal(dec)
    }

    pub fn checked_add(self, other: Self) -> Result<Self, CodecError> {
        self.ensure_finite()?;
        other.ensure_finite()?;
        let a = self.to_decimal_checked()?;
        let b = other.to_decimal_checked()?;
        let res = a
            .checked_add(b)
            .ok_or(CodecError::Fixed8ArithmeticOverflow)?;
        Self::from_decimal(res)
    }

    pub fn checked_sub(self, other: Self) -> Result<Self, CodecError> {
        self.ensure_finite()?;
        other.ensure_finite()?;
        let a = self.to_decimal_checked()?;
        let b = other.to_decimal_checked()?;
        let res = a
            .checked_sub(b)
            .ok_or(CodecError::Fixed8ArithmeticOverflow)?;
        Self::from_decimal(res)
    }

    pub fn checked_neg(self) -> Result<Self, CodecError> {
        self.ensure_finite()?;
        let a = self.to_decimal_checked()?;
        let res = Decimal::ZERO
            .checked_sub(a)
            .ok_or(CodecError::Fixed8ArithmeticOverflow)?;
        Self::from_decimal(res)
    }

    pub fn checked_abs(self) -> Result<Self, CodecError> {
        self.ensure_finite()?;
        let a = self.to_decimal_checked()?;
        let res = a.abs();
        Self::from_decimal(res)
    }

    pub fn checked_mul(self, other: Self) -> Result<Self, CodecError> {
        self.ensure_finite()?;
        other.ensure_finite()?;
        let a = self.to_decimal_checked()?;
        let b = other.to_decimal_checked()?;
        let res = a
            .checked_mul(b)
            .ok_or(CodecError::Fixed8ArithmeticOverflow)?;
        Self::from_decimal(res)
    }

    pub fn checked_div(self, other: Self) -> Result<Self, CodecError> {
        self.ensure_finite()?;
        other.ensure_finite()?;
        if other.is_zero() {
            return Err(CodecError::Fixed8DivisionByZero);
        }
        let a = self.to_decimal_checked()?;
        let b = other.to_decimal_checked()?;
        let res = a
            .checked_div(b)
            .ok_or(CodecError::Fixed8ArithmeticOverflow)?;
        Self::from_decimal(res)
    }

    pub fn truncate_to_increment(self, increment: Self) -> Result<Self, CodecError> {
        self.ensure_finite()?;
        increment.ensure_finite()?;
        if increment.is_zero() || increment.scaled <= 0 {
            return Err(CodecError::InvalidFixed8Increment);
        }
        let val_dec = self.to_decimal_checked()?;
        let inc_dec = increment.to_decimal_checked()?;
        let res = truncate_decimal_to_increment(val_dec, inc_dec)?;
        Self::from_decimal(res)
    }

    pub fn ceil_to_increment(self, increment: Self) -> Result<Self, CodecError> {
        self.ensure_finite()?;
        increment.ensure_finite()?;
        if increment.is_zero() || increment.scaled <= 0 {
            return Err(CodecError::InvalidFixed8Increment);
        }
        let val_dec = self.to_decimal_checked()?;
        let inc_dec = increment.to_decimal_checked()?;
        let res = ceil_decimal_to_increment(val_dec, inc_dec)?;
        Self::from_decimal(res)
    }

    fn ensure_finite(self) -> Result<(), CodecError> {
        if self.non_finite != 0 {
            Err(CodecError::Fixed8NonFiniteArithmetic)
        } else {
            Ok(())
        }
    }

    pub fn storage_text(self) -> String {
        if self.non_finite == 1 {
            return "inf".to_owned();
        }
        if self.non_finite == -1 {
            return "-inf".to_owned();
        }
        self.fixed_text()
            .trim_end_matches('0')
            .trim_end_matches('.')
            .to_owned()
    }

    pub fn fixed_text(self) -> String {
        if self.non_finite == 1 {
            return "inf".to_owned();
        }
        if self.non_finite == -1 {
            return "-inf".to_owned();
        }
        let sign = if self.scaled < 0 { "-" } else { "" };
        let magnitude = self.scaled.unsigned_abs();
        format!("{sign}{}.{:08}", magnitude / SCALE, magnitude % SCALE)
    }
}

impl From<Fixed8> for Decimal {
    fn from(value: Fixed8) -> Self {
        value.to_decimal()
    }
}

impl From<Decimal> for Fixed8 {
    fn from(value: Decimal) -> Self {
        Self::from_decimal(value).unwrap_or_else(|_| {
            if value.is_sign_negative() {
                Self::NEG_INFINITY
            } else {
                Self::POS_INFINITY
            }
        })
    }
}

/// Trading math helper extension on [`Decimal`].
pub trait DecimalTradingExt {
    /// Truncates `self` down to the nearest positive increment.
    fn truncate_to_increment(self, increment: Decimal) -> Result<Decimal, CodecError>;

    /// Ceils `self` up to the nearest positive increment.
    fn ceil_to_increment(self, increment: Decimal) -> Result<Decimal, CodecError>;

    /// Aligns `self` to the nearest step (rounding half to even / nearest).
    fn align_to_step(self, step: Decimal) -> Decimal;

    /// Formats decimal as canonical storage text (trimming trailing zeros after decimal point).
    fn to_storage_text(self) -> String;

    /// Parses percentage or decimal string (e.g. "12.5%", "100.25").
    fn parse_trading_str(input: &str) -> Result<Decimal, CodecError>;
}

impl DecimalTradingExt for Decimal {
    fn truncate_to_increment(self, increment: Decimal) -> Result<Decimal, CodecError> {
        truncate_decimal_to_increment(self, increment)
    }

    fn ceil_to_increment(self, increment: Decimal) -> Result<Decimal, CodecError> {
        ceil_decimal_to_increment(self, increment)
    }

    fn align_to_step(self, step: Decimal) -> Decimal {
        align_decimal_to_step(self, step)
    }

    fn to_storage_text(self) -> String {
        self.normalize().to_string()
    }

    fn parse_trading_str(input: &str) -> Result<Decimal, CodecError> {
        let trimmed = input.trim();
        if trimmed.is_empty() {
            return Ok(Decimal::ZERO);
        }
        if let Some(pct) = trimmed.strip_suffix('%') {
            let val: Decimal = pct
                .parse()
                .map_err(|_| CodecError::InvalidDecimal(input.to_owned()))?;
            val.checked_div(Decimal::from(100))
                .ok_or(CodecError::Fixed8ArithmeticOverflow)
        } else {
            trimmed
                .parse()
                .map_err(|_| CodecError::InvalidDecimal(input.to_owned()))
        }
    }
}

/// Truncates `value` to a positive `increment`.
pub fn truncate_decimal_to_increment(
    value: Decimal,
    increment: Decimal,
) -> Result<Decimal, CodecError> {
    if increment <= Decimal::ZERO {
        return Err(CodecError::InvalidFixed8Increment);
    }
    let quotient = value
        .checked_div(increment)
        .ok_or(CodecError::Fixed8ArithmeticOverflow)?;
    let units = quotient.trunc();
    units
        .checked_mul(increment)
        .ok_or(CodecError::Fixed8ArithmeticOverflow)
}

/// Ceils `value` to a positive `increment`.
pub fn ceil_decimal_to_increment(
    value: Decimal,
    increment: Decimal,
) -> Result<Decimal, CodecError> {
    if increment <= Decimal::ZERO {
        return Err(CodecError::InvalidFixed8Increment);
    }
    let quotient = value
        .checked_div(increment)
        .ok_or(CodecError::Fixed8ArithmeticOverflow)?;
    let units = quotient.ceil();
    units
        .checked_mul(increment)
        .ok_or(CodecError::Fixed8ArithmeticOverflow)
}

/// Aligns `value` to the nearest positive `step`.
pub fn align_decimal_to_step(value: Decimal, step: Decimal) -> Decimal {
    if step.is_zero() {
        return value;
    }
    let Some(quotient) = value.checked_div(step) else {
        return value;
    };
    let rounded = quotient.round();
    rounded.checked_mul(step).unwrap_or(value)
}

impl fmt::Display for Fixed8 {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.storage_text())
    }
}

impl FromStr for Fixed8 {
    type Err = CodecError;

    fn from_str(input: &str) -> Result<Self, Self::Err> {
        if input.is_empty() {
            return Ok(Self::ZERO);
        }
        match input.to_ascii_lowercase().as_str() {
            "inf" | "+inf" => return Ok(Self::POS_INFINITY),
            "-inf" => return Ok(Self::NEG_INFINITY),
            _ => {}
        }
        let (numeric, percentage) = input
            .strip_suffix('%')
            .map_or((input, false), |value| (value, true));
        let mut parsed = parse_decimal(numeric)?;
        if percentage {
            parsed.scale = parsed
                .scale
                .checked_add(2)
                .ok_or(CodecError::Fixed8OutOfRange)?;
        }
        scaled_value(parsed).map(Self::from_scaled)
    }
}

fn scaled_value(parsed: ParsedDecimal) -> Result<i64, CodecError> {
    let significant = parsed.digits.trim_start_matches('0');
    if significant.is_empty() {
        return Ok(0);
    }
    let shift = 8_i64
        .checked_sub(parsed.scale)
        .ok_or(CodecError::Fixed8OutOfRange)?;
    let magnitude_text = if shift >= 0 {
        let shift = usize::try_from(shift).map_err(|_| CodecError::Fixed8OutOfRange)?;
        if significant.len().saturating_add(shift) > 20 {
            return Err(CodecError::Fixed8OutOfRange);
        }
        format!("{significant}{}", "0".repeat(shift))
    } else {
        let discarded = usize::try_from(-shift).map_err(|_| CodecError::Fixed8OutOfRange)?;
        if discarded >= significant.len() {
            return Ok(0);
        }
        significant[..significant.len() - discarded].to_owned()
    };
    let magnitude = magnitude_text
        .parse::<u64>()
        .map_err(|_| CodecError::Fixed8OutOfRange)?;
    if parsed.negative {
        if magnitude > (i64::MAX as u64) + 1 {
            return Err(CodecError::Fixed8OutOfRange);
        }
        i64::try_from(-(i128::from(magnitude))).map_err(|_| CodecError::Fixed8OutOfRange)
    } else {
        i64::try_from(magnitude).map_err(|_| CodecError::Fixed8OutOfRange)
    }
}

impl Serialize for Fixed8 {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        if self.non_finite != 0 {
            return serializer.serialize_str(&self.storage_text());
        }
        let number =
            serde_json::Number::from_str(&self.fixed_text()).map_err(serde::ser::Error::custom)?;
        number.serialize(serializer)
    }
}

#[derive(Deserialize)]
#[serde(untagged)]
enum Fixed8Representation {
    Text(String),
    Number(serde_json::Number),
    Null,
}

impl<'de> Deserialize<'de> for Fixed8 {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        let input = match Fixed8Representation::deserialize(deserializer)? {
            Fixed8Representation::Text(value) => value,
            Fixed8Representation::Number(value) => value.to_string(),
            Fixed8Representation::Null => String::new(),
        };
        input.parse().map_err(serde::de::Error::custom)
    }
}

#[cfg(test)]
mod tests {
    use super::{DecimalTradingExt, Fixed8};
    use rust_decimal::Decimal;
    use std::str::FromStr;

    #[test]
    fn rejects_values_outside_the_fixed_width() {
        assert!("92233720369".parse::<Fixed8>().is_err());
        assert!("-92233720369".parse::<Fixed8>().is_err());
    }

    #[test]
    fn checked_arithmetic_preserves_eight_decimal_truncation() {
        let three = "3".parse::<Fixed8>().expect("three");
        let ten = "10".parse::<Fixed8>().expect("ten");
        let third = three.checked_div(ten).expect("division");
        assert_eq!(third.storage_text(), "0.3");
        assert_eq!(third.checked_mul(ten).expect("multiplication"), three);
        assert_eq!(
            "1.239"
                .parse::<Fixed8>()
                .expect("quantity")
                .truncate_to_increment("0.01".parse().expect("increment"))
                .expect("truncate")
                .storage_text(),
            "1.23"
        );
        assert_eq!(
            "1.001"
                .parse::<Fixed8>()
                .expect("fee")
                .ceil_to_increment("0.01".parse().expect("cent"))
                .expect("ceil")
                .storage_text(),
            "1.01"
        );
    }

    #[test]
    fn conversions_between_fixed8_and_decimal() {
        let f = "100.25".parse::<Fixed8>().expect("fixed8");
        let d: Decimal = f.into();
        assert_eq!(d, Decimal::from_str("100.25").unwrap());
        assert_eq!(f.to_decimal(), Decimal::from_str("100.25").unwrap());

        let back: Fixed8 = d.into();
        assert_eq!(back, f);
        assert_eq!(Fixed8::from_decimal(d).unwrap(), f);

        assert_eq!(Fixed8::POS_INFINITY.to_decimal(), Decimal::MAX);
        assert_eq!(Fixed8::NEG_INFINITY.to_decimal(), Decimal::MIN);
        assert_eq!(Fixed8::from(Decimal::MAX), Fixed8::POS_INFINITY);
        assert_eq!(Fixed8::from(Decimal::MIN), Fixed8::NEG_INFINITY);
    }

    #[test]
    fn decimal_trading_ext_helpers() {
        let val = Decimal::from_str("1.239").unwrap();
        let step = Decimal::from_str("0.01").unwrap();
        assert_eq!(
            val.truncate_to_increment(step).unwrap(),
            Decimal::from_str("1.23").unwrap()
        );
        assert_eq!(
            Decimal::from_str("1.001")
                .unwrap()
                .ceil_to_increment(step)
                .unwrap(),
            Decimal::from_str("1.01").unwrap()
        );
        assert_eq!(
            Decimal::from_str("1.004")
                .unwrap()
                .align_to_step(Decimal::from_str("0.005").unwrap()),
            Decimal::from_str("1.005").unwrap()
        );
        assert_eq!(
            Decimal::from_str("100.250000").unwrap().to_storage_text(),
            "100.25"
        );
        assert_eq!(
            Decimal::parse_trading_str("12.5%").unwrap(),
            Decimal::from_str("0.125").unwrap()
        );
        assert_eq!(
            Decimal::parse_trading_str(" 100.5 ").unwrap(),
            Decimal::from_str("100.5").unwrap()
        );
    }
}
