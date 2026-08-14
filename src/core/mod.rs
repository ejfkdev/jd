// core — core abstractions for the jd multi-language deobfuscation framework.
// Reference: disrobe-core's Pass/Detector/Registry/Chain architecture.

pub mod pass;
pub mod artifact;
pub mod detect;
pub mod registry;
pub mod chain;

pub use pass::{Pass, Detector, PassId};
pub use artifact::{Artifact, OutputKind, Language, ChildArtifact, ChildHandle};
pub use detect::{DetectContext, DetectVerdict};
pub use registry::{PassRegistry, DetectorPick};
pub use chain::{ChainSpec, ChainDriver, ChainPlan, RecoveredFile};
