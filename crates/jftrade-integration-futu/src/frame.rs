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
    fn encode_decode_round_trip_matches_opend_wire() {
        // Parity: go:452dea11:pkg/futu/codec/frame_test.go:8 TestEncodeDecodeRoundTrip
        let packet = encode_frame(1001, 42, &[1, 2, 3, 4, 5]).expect("encode");
        assert_eq!(packet.len(), HEADER_LEN + 5);
        let frame = decode_frame(&packet).expect("decode");
        assert_eq!(frame.header.proto_id, 1001);
        assert_eq!(frame.header.serial_no, 42);
        assert_eq!(frame.body, [1, 2, 3, 4, 5]);
    }

    #[test]
    fn decode_rejects_corrupted_body_hash() {
        // Parity: go:452dea11:pkg/futu/codec/frame_test.go:29 TestDecodeRejectsCorruptedBody
        let mut packet = encode_frame(2001, 1, b"hello").expect("encode");
        *packet.last_mut().expect("body") ^= 0xff;
        assert_eq!(decode_frame(&packet), Err(FrameError::BadBodyHash));
    }

    #[test]
    fn decode_rejects_bad_magic() {
        // Parity: go:452dea11:pkg/futu/codec/frame_test.go:39 TestDecodeRejectsBadMagic
        let mut packet = encode_frame(2001, 1, &[0]).expect("encode");
        packet[0] = b'X';
        assert_eq!(decode_frame(&packet), Err(FrameError::BadMagic));
    }

    #[test]
    fn decode_rejects_short_frame() {
        // Parity: go:452dea11:pkg/futu/codec/frame_test.go:49 TestDecodeRejectsShortFrame
        assert_eq!(decode_frame(&[1, 2, 3]), Err(FrameError::TooShort));
    }

    #[test]
    fn decode_rejects_length_mismatch() {
        // Parity: go:452dea11:pkg/futu/codec/frame_test.go:56 TestDecodeRejectsLengthMismatch
        let packet = encode_frame(1, 1, &[1, 2, 3]).expect("encode");
        assert_eq!(
            decode_frame(&packet[..HEADER_LEN + 1]),
            Err(FrameError::LengthMismatch {
                declared: 3,
                actual: 1,
            })
        );
    }

    #[test]
    fn frame_size_guards_cover_encode_and_decode() {
        // Parity: go:452dea11:pkg/futu/codec/frame_size_guards_test.go:9 TestFrameSizeGuardsCoverEncodeAndDecode
        assert_eq!(
            encode_frame(1, 1, &vec![0_u8; MAX_BODY_LEN + 1]),
            Err(FrameError::BodyTooLarge)
        );

        // The same guard must reject an oversized declared body before the
        // reader allocates a buffer for it.
        let mut oversized_header = [0_u8; HEADER_LEN];
        oversized_header[0] = b'F';
        oversized_header[1] = b'T';
        oversized_header[12..16].copy_from_slice(&(MAX_BODY_LEN as u32 + 1).to_le_bytes());
        assert_eq!(
            decode_frame(&oversized_header),
            Err(FrameError::BodyTooLarge)
        );
    }
}
