use std::fmt;
use std::str::FromStr;

use serde::{Deserialize, Deserializer, Serialize, Serializer};

use crate::CodecError;

const MAX_EXPANDED_DIGITS: i64 = 1_000_000;

#[derive(Clone, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct DecimalText(String);

#[derive(Debug)]
pub(crate) struct ParsedDecimal {
    pub(crate) negative: bool,
    pub(crate) digits: String,
    pub(crate) scale: i64,
}

impl DecimalText {
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl fmt::Display for DecimalText {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.0)
    }
}

impl FromStr for DecimalText {
    type Err = CodecError;

    fn from_str(input: &str) -> Result<Self, Self::Err> {
        canonical_decimal(input).map(Self)
    }
}

impl Serialize for DecimalText {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        serializer.serialize_str(&self.0)
    }
}

#[derive(Deserialize)]
#[serde(untagged)]
enum DecimalRepresentation {
    Text(String),
    Number(serde_json::Number),
    Null,
}

impl<'de> Deserialize<'de> for DecimalText {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        let input = match DecimalRepresentation::deserialize(deserializer)? {
            DecimalRepresentation::Text(value) => value,
            DecimalRepresentation::Number(value) => value.to_string(),
            DecimalRepresentation::Null => "0".to_owned(),
        };
        input.parse().map_err(serde::de::Error::custom)
    }
}

pub(crate) fn parse_decimal(input: &str) -> Result<ParsedDecimal, CodecError> {
    if input.is_empty() || input.trim() != input {
        return Err(CodecError::InvalidDecimal(input.to_owned()));
    }
    let (negative, unsigned) = match input.as_bytes()[0] {
        b'-' => (true, &input[1..]),
        b'+' => (false, &input[1..]),
        _ => (false, input),
    };
    if unsigned.is_empty() {
        return Err(CodecError::InvalidDecimal(input.to_owned()));
    }
    let mut exponent_parts = unsigned.split(['e', 'E']);
    let coefficient = exponent_parts.next().unwrap_or_default();
    let exponent = match exponent_parts.next() {
        Some(value) if !value.is_empty() => value
            .parse::<i64>()
            .map_err(|_| CodecError::InvalidDecimal(input.to_owned()))?,
        Some(_) => return Err(CodecError::InvalidDecimal(input.to_owned())),
        None => 0,
    };
    if exponent_parts.next().is_some() || exponent.unsigned_abs() > MAX_EXPANDED_DIGITS as u64 {
        return Err(CodecError::DecimalExpansionLimit);
    }

    let mut decimal_parts = coefficient.split('.');
    let integer = decimal_parts.next().unwrap_or_default();
    let fraction = decimal_parts.next();
    if decimal_parts.next().is_some()
        || (integer.is_empty() && fraction.is_none_or(str::is_empty))
        || !integer.bytes().all(|byte| byte.is_ascii_digit())
        || fraction.is_some_and(|value| !value.bytes().all(|byte| byte.is_ascii_digit()))
    {
        return Err(CodecError::InvalidDecimal(input.to_owned()));
    }
    let fraction = fraction.unwrap_or_default();
    let digits = format!("{integer}{fraction}");
    if digits.is_empty() {
        return Err(CodecError::InvalidDecimal(input.to_owned()));
    }
    Ok(ParsedDecimal {
        negative,
        digits,
        scale: i64::try_from(fraction.len())
            .map_err(|_| CodecError::DecimalExpansionLimit)?
            .checked_sub(exponent)
            .ok_or(CodecError::DecimalExpansionLimit)?,
    })
}

fn canonical_decimal(input: &str) -> Result<String, CodecError> {
    let ParsedDecimal {
        negative,
        digits,
        scale,
    } = parse_decimal(input)?;
    let significant = digits.trim_start_matches('0');
    if significant.is_empty() {
        return Ok("0".to_owned());
    }
    let significant_length =
        i64::try_from(significant.len()).map_err(|_| CodecError::DecimalExpansionLimit)?;
    let output_length = significant_length
        .checked_add(
            i64::try_from(scale.unsigned_abs()).map_err(|_| CodecError::DecimalExpansionLimit)?,
        )
        .ok_or(CodecError::DecimalExpansionLimit)?;
    if output_length > MAX_EXPANDED_DIGITS {
        return Err(CodecError::DecimalExpansionLimit);
    }

    let mut value = if scale <= 0 {
        let zeros = usize::try_from(-scale).map_err(|_| CodecError::DecimalExpansionLimit)?;
        format!("{significant}{}", "0".repeat(zeros))
    } else {
        let scale = usize::try_from(scale).map_err(|_| CodecError::DecimalExpansionLimit)?;
        if significant.len() > scale {
            let split = significant.len() - scale;
            let fraction = significant[split..].trim_end_matches('0');
            if fraction.is_empty() {
                significant[..split].to_owned()
            } else {
                format!("{}.{}", &significant[..split], fraction)
            }
        } else {
            let fraction = format!("{}{}", "0".repeat(scale - significant.len()), significant)
                .trim_end_matches('0')
                .to_owned();
            format!("0.{fraction}")
        }
    };
    if negative {
        value.insert(0, '-');
    }
    Ok(value)
}

#[cfg(test)]
mod tests {
    use super::DecimalText;

    #[test]
    fn rejects_ambiguous_or_unbounded_decimal_inputs() {
        for input in ["", " 1", ".", "1e", "1.2.3", "1e1000001"] {
            assert!(input.parse::<DecimalText>().is_err(), "{input}");
        }
    }
}
