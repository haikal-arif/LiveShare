use std::collections::VecDeque;
use std::time::Instant;

use rtc::interceptor::{Attribute, Interceptor, Packet, StreamInfo, TaggedPacket};
use rtc::rtcp::payload_feedbacks::full_intra_request::FullIntraRequest;
use rtc::rtcp::payload_feedbacks::picture_loss_indication::PictureLossIndication;

/// Whether an RTP payload starts a keyframe. Codecs without a parser here
/// are assumed to, so switches still happen (possibly with a brief glitch).
pub(super) fn is_keyframe(mime_type: &str, payload: &[u8]) -> bool {
    let mime = mime_type.to_ascii_lowercase();
    match mime.as_str() {
        "video/vp8" => is_vp8_keyframe(payload),
        // VP9 payload descriptor, first octet: 0x08 = B (start of a frame),
        // 0x40 = P (inter-picture predicted; clear on a keyframe).
        // Spec: https://datatracker.ietf.org/doc/draft-ietf-payload-vp9/ (draft, not an RFC)
        "video/vp9" => payload
            .first()
            .is_some_and(|b| b & 0x40 == 0 && b & 0x08 != 0),
        "video/h264" => is_h264_keyframe(payload),
        _ => true,
    }
}

/// RFC 7741: start of partition 0, then the VP8 frame header's P bit clear.
/// Spec: https://www.rfc-editor.org/rfc/rfc7741
fn is_vp8_keyframe(payload: &[u8]) -> bool {
    let Some(&b0) = payload.first() else {
        return false;
    };
    // Descriptor octet: 0x10 = S (start of a partition), 0x07 = PartID
    // (must be 0 for partition 0, where the frame header lives).
    if b0 & 0x10 == 0 || b0 & 0x07 != 0 {
        return false;
    }
    let mut i = 1;
    // 0x80 = X: an extension octet follows the descriptor.
    if b0 & 0x80 != 0 {
        let Some(&x) = payload.get(1) else {
            return false;
        };
        i = 2;
        // Extension octet: 0x80 = I (PictureID present), 0x40 = L (TL0PICIDX
        // present), 0x30 = T or K (TID/KEYIDX present). Each present field
        // is skipped below, so `i` lands on the frame header.
        if x & 0x80 != 0 {
            // PictureID: one byte, or two when the M bit (0x80 of its first
            // octet) is set, which gives a 15-bit ID.
            i += if payload.get(i).is_some_and(|b| b & 0x80 != 0) {
                2
            } else {
                1
            };
        }
        if x & 0x40 != 0 {
            i += 1;
        }
        if x & 0x30 != 0 {
            i += 1;
        }
    }
    // Frame header, first octet: 0x01 = P (inter frame). Clear means keyframe.
    payload.get(i).is_some_and(|b| b & 0x01 == 0)
}

/// RFC 6184: an IDR slice or SPS, directly, inside STAP-A, or as the first
/// FU-A fragment. Spec: https://www.rfc-editor.org/rfc/rfc6184
fn is_h264_keyframe(payload: &[u8]) -> bool {
    // NAL unit type is the low 5 bits (0x1F): 5 = IDR slice, 7 = SPS.
    let is_key_nal = |nal: u8| matches!(nal & 0x1F, 5 | 7);
    let Some(&b0) = payload.first() else {
        return false;
    };
    match b0 & 0x1F {
        // 24 = STAP-A: one header octet, then NAL units, each prefixed by a
        // 16-bit big-endian size. The NAL header sits at `i + 2`.
        24 => {
            let mut i = 1;
            while i + 2 < payload.len() {
                let size = u16::from_be_bytes([payload[i], payload[i + 1]]) as usize;
                if is_key_nal(payload[i + 2]) {
                    return true;
                }
                i += 2 + size;
            }
            false
        }
        // 28 = FU-A: the second octet is the FU header, where 0x80 = S (first
        // fragment) and the low 5 bits repeat the fragmented NAL type.
        28 => payload
            .get(1)
            .is_some_and(|b| b & 0x80 != 0 && is_key_nal(*b)),
        _ => is_key_nal(b0),
    }
}

pub(super) fn is_keyframe_request(packet: &dyn rtc::rtcp::Packet) -> bool {
    let packet = packet.as_any();
    packet.is::<PictureLossIndication>() || packet.is::<FullIntraRequest>()
}

/// Inbound RTCP stops at the end of the interceptor chain unless tagged for
/// the application; this tags keyframe requests so senders can see them.
#[derive(Default)]
pub(super) struct KeyframeRequestTagger {
    read_queue: VecDeque<TaggedPacket>,
    write_queue: VecDeque<TaggedPacket>,
}

impl rtc::sansio::Protocol<TaggedPacket, TaggedPacket, ()> for KeyframeRequestTagger {
    type Rout = TaggedPacket;
    type Wout = TaggedPacket;
    type Eout = ();
    type Error = rtc::shared::error::Error;
    type Time = Instant;

    fn handle_read(&mut self, mut msg: TaggedPacket) -> Result<(), Self::Error> {
        if let Packet::Rtcp(packets) = &msg.message.packet {
            if packets.iter().any(|p| is_keyframe_request(p.as_ref())) {
                msg.message.add(Attribute::DeliverToApplication);
            }
        }
        self.read_queue.push_back(msg);
        Ok(())
    }

    fn poll_read(&mut self) -> Option<Self::Rout> {
        self.read_queue.pop_front()
    }

    fn handle_write(&mut self, msg: TaggedPacket) -> Result<(), Self::Error> {
        self.write_queue.push_back(msg);
        Ok(())
    }

    fn poll_write(&mut self) -> Option<Self::Wout> {
        self.write_queue.pop_front()
    }
}

impl Interceptor for KeyframeRequestTagger {
    fn bind_local_stream(&mut self, _info: &StreamInfo) {}
    fn unbind_local_stream(&mut self, _info: &StreamInfo) {}
    fn bind_remote_stream(&mut self, _info: &StreamInfo) {}
    fn unbind_remote_stream(&mut self, _info: &StreamInfo) {}
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn detects_vp8_keyframes() {
        assert!(is_keyframe("video/VP8", &[0x10, 0x00]));
        assert!(!is_keyframe("video/VP8", &[0x10, 0x01]), "inter frame");
        assert!(
            !is_keyframe("video/VP8", &[0x00, 0x00]),
            "not partition start"
        );
        // X, I with 15-bit PictureID, then the frame header.
        assert!(is_keyframe("video/VP8", &[0x90, 0x80, 0x81, 0x23, 0x00]));
        assert!(!is_keyframe("video/VP8", &[0x90, 0x80, 0x81, 0x23, 0x01]));
    }

    #[test]
    fn detects_h264_keyframes() {
        assert!(is_keyframe("video/H264", &[0x65]), "IDR");
        assert!(!is_keyframe("video/H264", &[0x41]), "non-IDR slice");
        assert!(
            is_keyframe("video/H264", &[0x18, 0x00, 0x01, 0x67, 0x00, 0x01, 0x68]),
            "STAP-A with SPS"
        );
        assert!(
            is_keyframe("video/H264", &[0x7C, 0x85]),
            "FU-A start of IDR"
        );
        assert!(
            !is_keyframe("video/H264", &[0x7C, 0x05]),
            "FU-A continuation"
        );
    }

    #[test]
    fn detects_vp9_keyframes() {
        assert!(
            is_keyframe("video/VP9", &[0x08]),
            "start of frame, not inter-predicted"
        );
        assert!(
            !is_keyframe("video/VP9", &[0x48]),
            "P bit set: inter-picture predicted"
        );
        assert!(!is_keyframe("video/VP9", &[0x00]), "not start of frame");
        assert!(!is_keyframe("video/VP9", &[]), "empty payload");
    }

    #[test]
    fn vp8_skips_each_optional_header_field() {
        assert!(!is_keyframe("video/VP8", &[]), "empty payload");
        assert!(
            !is_keyframe("video/VP8", &[0x11, 0x00]),
            "PartID 1 is not partition 0"
        );
        assert!(
            !is_keyframe("video/VP8", &[0x90]),
            "X set but extension octet missing"
        );
        assert!(
            is_keyframe("video/VP8", &[0x90, 0x80, 0x23, 0x00]),
            "7-bit PictureID skipped"
        );
        assert!(
            !is_keyframe("video/VP8", &[0x90, 0x80, 0x23, 0x01]),
            "7-bit PictureID, inter frame"
        );
        assert!(
            !is_keyframe("video/VP8", &[0x90, 0x80, 0x23]),
            "frame header cut off"
        );
        assert!(
            is_keyframe("video/VP8", &[0x90, 0x40, 0x00, 0x00]),
            "TL0PICIDX skipped"
        );
        assert!(
            is_keyframe("video/VP8", &[0x90, 0x20, 0x00, 0x00]),
            "TID/KEYIDX skipped"
        );
    }

    #[test]
    fn h264_stap_a_checks_every_aggregated_nal() {
        assert!(!is_keyframe("video/H264", &[]), "empty payload");
        assert!(
            !is_keyframe("video/H264", &[0x18, 0x00, 0x01, 0x41]),
            "STAP-A holding only a non-IDR slice"
        );
        assert!(
            is_keyframe("video/H264", &[0x18, 0x00, 0x01, 0x41, 0x00, 0x01, 0x65]),
            "IDR is the second NAL in STAP-A"
        );
    }

    #[test]
    fn keyframe_requests_are_pli_and_fir_only() {
        let pli = PictureLossIndication {
            sender_ssrc: 1,
            media_ssrc: 2,
        };
        let fir = FullIntraRequest {
            sender_ssrc: 1,
            media_ssrc: 2,
            fir: vec![],
        };
        assert!(is_keyframe_request(&pli), "PLI");
        assert!(is_keyframe_request(&fir), "FIR");
        assert!(
            !is_keyframe_request(&rtc::rtcp::receiver_report::ReceiverReport::default()),
            "receiver report is not a keyframe request"
        );
    }

    #[test]
    fn tagger_marks_only_keyframe_requests_for_the_application() {
        use rtc::interceptor::AttributedPacket;
        use rtc::sansio::Protocol;
        use rtc::shared::{TransportContext, TransportProtocol};

        let transport = || TransportContext {
            local_addr: "127.0.0.1:9000".parse().unwrap(),
            peer_addr: "127.0.0.1:50000".parse().unwrap(),
            transport_protocol: TransportProtocol::UDP,
            ecn: None,
        };
        let pli = Packet::Rtcp(vec![Box::new(PictureLossIndication {
            sender_ssrc: 1,
            media_ssrc: 2,
        })]);
        let rr = Packet::Rtcp(vec![Box::new(
            rtc::rtcp::receiver_report::ReceiverReport::default(),
        )]);

        let mut tagger = KeyframeRequestTagger::default();
        tagger
            .handle_read(TaggedPacket {
                now: Instant::now(),
                transport: transport(),
                message: AttributedPacket::new(pli),
            })
            .unwrap();
        tagger
            .handle_read(TaggedPacket {
                now: Instant::now(),
                transport: transport(),
                message: AttributedPacket::new(rr),
            })
            .unwrap();

        let first = tagger.poll_read().expect("PLI is queued");
        assert!(
            first
                .message
                .attributes
                .iter()
                .any(|a| matches!(a, Attribute::DeliverToApplication)),
            "PLI is tagged for the application"
        );
        let second = tagger.poll_read().expect("receiver report is queued");
        assert!(
            second.message.attributes.is_empty(),
            "receiver report is not tagged"
        );
        assert!(tagger.poll_read().is_none());
    }

    #[test]
    fn stream_binding_hooks_do_nothing() {
        use rtc::interceptor::StreamInfo;

        let mut tagger = KeyframeRequestTagger::default();
        let info = StreamInfo::default();
        tagger.bind_local_stream(&info);
        tagger.unbind_local_stream(&info);
        tagger.bind_remote_stream(&info);
        tagger.unbind_remote_stream(&info);
        assert!(tagger.read_queue.is_empty() && tagger.write_queue.is_empty());
    }
}
