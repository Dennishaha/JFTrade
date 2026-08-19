use std::fmt;
use std::str::FromStr;

use serde::{Deserialize, Deserializer, Serialize, Serializer};

use crate::CodecError;
use crate::decimal::{ParsedDecimal, parse_decimal};

const SCALE: u64 = 100_000_000;

#[derive(Clone, Copy, Debug, Default, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct Fixed8(i64);

impl Fixed8 {
    pub const NEG_INFINITY: Self = Self(i64::MIN);
    pub const POS_INFINITY: Self = Self(i64::MAX);
    pub const ZERO: Self = Self(0);

    pub const fn from_scaled(scaled: i64) -> Self {
        Self(scaled)
    }

    pub const fn scaled(self) -> i64 {
        self.0
    }

    pub fn storage_text(self) -> String {
        if self == Self::POS_INFINITY {
            return "inf".to_owned();
        }
        if self == Self::NEG_INFINITY {
            return "-inf".to_owned();
        }
        self.fixed_text()
            .trim_end_matches('0')
            .trim_end_matches('.')
            .to_owned()
    }

    fn fixed_text(self) -> String {
        let sign = if self.0 < 0 { "-" } else { "" };
        let magnitude = self.0.unsigned_abs();
        format!("{sign}{}.{:08}", magnitude / SCALE, magnitude % SCALE)
    }
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
        scaled_value(parsed).map(Self)
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
        if *self == Self::POS_INFINITY || *self == Self::NEG_INFINITY {
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
    use super::Fixed8;

    #[test]
    fn rejects_values_outside_the_fixed_width() {
        assert!("92233720369".parse::<Fixed8>().is_err());
        assert!("-92233720369".parse::<Fixed8>().is_err());
    }
}
