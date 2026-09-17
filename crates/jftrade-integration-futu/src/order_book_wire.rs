//! Wire-level proto2 `required` enforcement for `Qot_GetOrderBook`.
//!
//! Go's `pkg/futu/opend/orderbook.go` decodes each nested message with
//! `proto.Unmarshal`, which rejects a payload whose proto2 `required` fields are
//! absent (`RequiredNotSet`). prost decodes the same payload into default values
//! and cannot observe presence afterwards, so the Rust boundary re-scans the
//! wire format and fails closed before the projection runs. Without this check a
//! truncated depth level would be published as `price=0, volume=0,
//! orderCount=0` instead of surfacing a decode error.

/// Returns `true` when `body` carries a field with `number` at the top level.
///
/// `body` must be a complete proto2 message body; malformed input is an error so
/// callers fail closed instead of trusting a partially scanned payload.
pub(crate) fn require_fields(body: &[u8], required: &[u32]) -> Result<(), String> {
    let fields = scan_fields(body)?;
    for number in required {
        if !fields.iter().any(|field| field.number == *number) {
            return Err(format!("required field {number} not set"));
        }
    }
    Ok(())
}

/// Validates the `required` fields of a `Qot_GetOrderBook` S2C payload.
///
/// Mirrors the two `proto.Unmarshal` calls in Go: the S2C `security` message
/// must carry `market`/`code`, and every ask/bid level must carry
/// `price`/`volume`/`orederCount`. Absent optional `s2c` and absent repeated
/// lists stay valid, matching the hand-written Go parser.
pub(crate) fn validate_order_book_s2c(body: &[u8]) -> Result<(), String> {
    for field in scan_fields(body)? {
        match field.number {
            // Qot_GetOrderBook.S2C.security (required Qot_Common.Security).
            1 => require_fields(length_delimited_payload(&field)?, &[1, 2])
                .map_err(|error| format!("unmarshal security: {error}"))?,
            // orderBookAskList / orderBookBidList (repeated Qot_Common.OrderBook).
            2 | 3 => require_fields(length_delimited_payload(&field)?, &[1, 2, 3])
                .map_err(|error| format!("unmarshal level: {error}"))?,
            _ => {}
        }
    }
    Ok(())
}

/// Validates a `Qot_GetOrderBook.Response` body before prost decodes it.
pub(crate) fn validate_order_book_response(body: &[u8]) -> Result<(), String> {
    for field in scan_fields(body)? {
        if field.number == 4 {
            validate_order_book_s2c(length_delimited_payload(&field)?)?;
        }
    }
    Ok(())
}

fn length_delimited_payload<'a>(field: &WireField<'a>) -> Result<&'a [u8], String> {
    if field.wire_type != 2 {
        return Err(format!("field {} must be length-delimited", field.number));
    }
    field
        .payload
        .ok_or_else(|| format!("field {} must be length-delimited", field.number))
}

#[derive(Clone, Copy, Debug)]
struct WireField<'a> {
    number: u32,
    wire_type: u8,
    payload: Option<&'a [u8]>,
}

/// Scans a proto2 message body into its top-level fields.
///
/// Supports the wire types OpenD emits (varint, fixed64, length-delimited and
/// fixed32); groups are rejected because no OpenD schema in this crate uses them.
fn scan_fields(mut body: &[u8]) -> Result<Vec<WireField<'_>>, String> {
    let mut fields = Vec::new();
    while !body.is_empty() {
        let (key, rest) = read_varint(body).ok_or_else(|| "truncated field tag".to_owned())?;
        let number = u32::try_from(key >> 3).map_err(|_| "field number overflow".to_owned())?;
        let wire_type = (key & 0x07) as u8;
        if number == 0 {
            return Err("invalid field number 0".to_owned());
        }
        body = rest;
        let (payload, rest) = match wire_type {
            0 => {
                let (_, rest) =
                    read_varint(body).ok_or_else(|| format!("invalid varint field {number}"))?;
                (None, rest)
            }
            1 => {
                if body.len() < 8 {
                    return Err(format!("invalid fixed64 field {number}"));
                }
                (None, &body[8..])
            }
            2 => {
                let (length, rest) =
                    read_varint(body).ok_or_else(|| format!("invalid bytes field {number}"))?;
                let length = usize::try_from(length)
                    .map_err(|_| format!("invalid bytes length on field {number}"))?;
                if rest.len() < length {
                    return Err(format!("invalid bytes field {number}"));
                }
                let (payload, rest) = rest.split_at(length);
                (Some(payload), rest)
            }
            5 => {
                if body.len() < 4 {
                    return Err(format!("invalid fixed32 field {number}"));
                }
                (None, &body[4..])
            }
            other => return Err(format!("unsupported wire type {other} on field {number}")),
        };
        fields.push(WireField {
            number,
            wire_type,
            payload,
        });
        body = rest;
    }
    Ok(fields)
}

fn read_varint(body: &[u8]) -> Option<(u64, &[u8])> {
    let mut value = 0_u64;
    for (index, byte) in body.iter().take(10).enumerate() {
        value |= u64::from(byte & 0x7f) << (7 * index);
        if byte & 0x80 == 0 {
            return Some((value, &body[index + 1..]));
        }
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;

    fn wire_varint(mut value: u64) -> Vec<u8> {
        let mut out = Vec::new();
        loop {
            let byte = (value & 0x7f) as u8;
            value >>= 7;
            if value == 0 {
                out.push(byte);
                return out;
            }
            out.push(byte | 0x80);
        }
    }

    fn wire_varint_field(number: u32, value: u64) -> Vec<u8> {
        let mut out = wire_varint(u64::from(number) << 3);
        out.extend(wire_varint(value));
        out
    }

    fn wire_bytes_field(number: u32, payload: &[u8]) -> Vec<u8> {
        let mut out = wire_varint((u64::from(number) << 3) | 2);
        out.extend(wire_varint(payload.len() as u64));
        out.extend_from_slice(payload);
        out
    }

    #[test]
    fn required_scan_reports_absent_and_malformed_fields() {
        let present = wire_varint_field(1, 7);
        assert!(require_fields(&present, &[1]).is_ok());
        assert_eq!(
            require_fields(&present, &[2]).expect_err("missing"),
            "required field 2 not set"
        );
        assert!(require_fields(&[0x80], &[1]).is_err());
    }

    #[test]
    fn depth_s2c_requires_security_and_level_fields() {
        let security = {
            let mut body = wire_varint_field(1, 11);
            body.extend(wire_bytes_field(2, b"AAPL"));
            body
        };
        let level = {
            let mut body = wire_varint((1 << 3) | 1);
            body.extend_from_slice(&320.5_f64.to_le_bytes());
            body.extend(wire_varint_field(2, 200));
            body.extend(wire_varint_field(3, 3));
            body
        };
        let s2c = {
            let mut body = wire_bytes_field(1, &security);
            body.extend(wire_bytes_field(2, &level));
            body
        };
        validate_order_book_s2c(&s2c).expect("well formed s2c");
        assert_eq!(
            validate_order_book_s2c(&wire_bytes_field(1, &[])).expect_err("empty security"),
            "unmarshal security: required field 1 not set"
        );
        assert_eq!(
            validate_order_book_s2c(&wire_bytes_field(2, &[])).expect_err("empty level"),
            "unmarshal level: required field 1 not set"
        );
    }
}
