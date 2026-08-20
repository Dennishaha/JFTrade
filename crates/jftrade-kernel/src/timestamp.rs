use std::fmt;
use std::str::FromStr;

use serde::{Deserialize, Deserializer, Serialize, Serializer};
use time::OffsetDateTime;
use time::format_description::well_known::Rfc3339;

use crate::CodecError;

#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct WireTimestamp(OffsetDateTime);

impl WireTimestamp {
    pub const fn from_offset_datetime(value: OffsetDateTime) -> Self {
        Self(value)
    }

    pub const fn into_inner(self) -> OffsetDateTime {
        self.0
    }

    pub fn unix_millis(self) -> Result<i64, CodecError> {
        i64::try_from(self.0.unix_timestamp_nanos().div_euclid(1_000_000))
            .map_err(|_| CodecError::TimestampOutOfRange)
    }

    fn wire_text(self) -> Result<String, CodecError> {
        self.0
            .format(&Rfc3339)
            .map_err(|error| CodecError::InvalidTimestamp(error.to_string()))
    }
}

impl fmt::Display for WireTimestamp {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        let value = self.wire_text().map_err(|_| fmt::Error)?;
        formatter.write_str(&value)
    }
}

impl FromStr for WireTimestamp {
    type Err = CodecError;

    fn from_str(input: &str) -> Result<Self, Self::Err> {
        OffsetDateTime::parse(input, &Rfc3339)
            .map(Self)
            .map_err(|error| CodecError::InvalidTimestamp(error.to_string()))
    }
}

impl Serialize for WireTimestamp {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        serializer.serialize_str(&self.wire_text().map_err(serde::ser::Error::custom)?)
    }
}

impl<'de> Deserialize<'de> for WireTimestamp {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        String::deserialize(deserializer)?
            .parse()
            .map_err(serde::de::Error::custom)
    }
}
