use sha1::{Digest, Sha1};
use thiserror::Error;

pub const HEADER_LEN: usize = 44;
pub const MAX_BODY_LEN: usize = 32 << 20;

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Header {
    pub proto_id: u32,
    pub proto_format: u8,
    pub proto_version: u8,
    pub serial_no: u32,
    pub body_len: u32,
    pub body_sha1: [u8; 20],
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Frame {
    pub header: Header,
    pub body: Vec<u8>,
}

#[derive(Clone, Debug, Eq, Error, PartialEq)]
pub enum FrameError {
    #[error("futu opend frame too short")]
    TooShort,
    #[error("futu opend frame has invalid magic")]
    BadMagic,
    #[error("futu opend frame body hash mismatch")]
    BadBodyHash,
    #[error("futu opend frame body too large")]
    BodyTooLarge,
    #[error("futu opend frame length mismatch: declared {declared}, actual {actual}")]
    LengthMismatch { declared: usize, actual: usize },
}

pub fn encode_frame(proto_id: u32, serial_no: u32, body: &[u8]) -> Result<Vec<u8>, FrameError> {
    if body.len() > MAX_BODY_LEN {
        return Err(FrameError::BodyTooLarge);
    }
    let mut packet = vec![0_u8; HEADER_LEN + body.len()];
    packet[0] = b'F';
    packet[1] = b'T';
    packet[2..6].copy_from_slice(&proto_id.to_le_bytes());
    packet[8..12].copy_from_slice(&serial_no.to_le_bytes());
    packet[12..16].copy_from_slice(&(body.len() as u32).to_le_bytes());
    let digest = Sha1::digest(body);
    packet[16..36].copy_from_slice(&digest);
    packet[HEADER_LEN..].copy_from_slice(body);
    Ok(packet)
}

pub fn decode_frame(packet: &[u8]) -> Result<Frame, FrameError> {
    if packet.len() < HEADER_LEN {
        return Err(FrameError::TooShort);
    }
    if packet[..2] != *b"FT" {
        return Err(FrameError::BadMagic);
    }
    let mut body_len = [0_u8; 4];
    body_len.copy_from_slice(&packet[12..16]);
    let declared = u32::from_le_bytes(body_len) as usize;
    if declared > MAX_BODY_LEN {
        return Err(FrameError::BodyTooLarge);
    }
    if packet.len() != HEADER_LEN + declared {
        return Err(FrameError::LengthMismatch {
            declared,
            actual: packet.len().saturating_sub(HEADER_LEN),
        });
    }
    let body = &packet[HEADER_LEN..];
    let actual_hash = Sha1::digest(body);
    if packet[16..36] != actual_hash[..] {
        return Err(FrameError::BadBodyHash);
    }
    let mut body_sha1 = [0_u8; 20];
    body_sha1.copy_from_slice(&packet[16..36]);
    let mut proto_id = [0_u8; 4];
    proto_id.copy_from_slice(&packet[2..6]);
    let mut serial_no = [0_u8; 4];
    serial_no.copy_from_slice(&packet[8..12]);
    Ok(Frame {
        header: Header {
            proto_id: u32::from_le_bytes(proto_id),
            proto_format: packet[6],
            proto_version: packet[7],
            serial_no: u32::from_le_bytes(serial_no),
            body_len: declared as u32,
            body_sha1,
        },
        body: body.to_vec(),
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn frame_round_trip_and_corruption_guards_match_opend_wire() {
        let packet = encode_frame(1001, 42, &[1, 2, 3, 4, 5]).expect("encode");
        let frame = decode_frame(&packet).expect("decode");
        assert_eq!(frame.header.proto_id, 1001);
        assert_eq!(frame.header.serial_no, 42);
        assert_eq!(frame.body, [1, 2, 3, 4, 5]);

        let mut corrupted = packet;
        *corrupted.last_mut().expect("body") ^= 0xff;
        assert_eq!(decode_frame(&corrupted), Err(FrameError::BadBodyHash));
    }
}
