//! The real WebRTC media plane, built on `webrtc-rs`.
//!
//! Each publisher has one `PeerConnection` with the SFU that only receives;
//! each (subscriber, publisher) pairing has its own `PeerConnection` that
//! only sends. Received RTP is fanned out per track over a `broadcast`
//! channel, and each subscriber pairing runs one relay task per track that
//! re-stamps packets for its own negotiated SSRC/payload type.
//!
//! A video track may arrive as several simulcast layers (rids `h`/`m`/`l`);
//! each relay forwards exactly one of them, chosen by its pairing's target
//! layer, and only switches at a keyframe of the new layer.

mod engine;
mod keyframe;
mod layer_switcher;
mod published_track;
mod relay;
mod session;

pub use engine::MediaContext;
