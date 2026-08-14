// Pass/Detector traits — the core plugin abstractions.
use super::artifact::{Artifact, OutputKind, ChildArtifact};
use super::detect::{DetectContext, DetectVerdict};

pub type PassId = &'static str;

pub trait Detector: Send + Sync {
    fn id(&self) -> PassId;
    fn detect(&self, ctx: &DetectContext) -> Option<DetectVerdict>;
}

pub trait Pass: Send + Sync {
    fn id(&self) -> PassId;
    fn detector(&self) -> &'static dyn Detector;
    fn output_kind(&self, output: &Artifact) -> OutputKind;
    fn run(&self, artifact: &Artifact) -> Result<Artifact, String>;
    fn extract_children(&self, _input: &Artifact) -> Result<Vec<ChildArtifact>, String> { Ok(Vec::new()) }
}
