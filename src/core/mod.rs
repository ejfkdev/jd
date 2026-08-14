// core — core abstractions for the jd multi-language deobfuscation framework.
// These traits allow jd to support multiple languages via a plugin system.
// Reference: disrobe-core's Pass/Detector/Registry architecture.

pub mod pass;
pub mod artifact;
pub mod detect;
pub mod registry;

pub use pass::{Pass, Detector, PassId};
pub use artifact::{Artifact, OutputKind, Language};
pub use detect::{DetectContext, DetectVerdict};
pub use registry::PassRegistry;
