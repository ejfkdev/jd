// passes/js — JavaScript deobfuscation pass.
// Wraps existing jd JS logic (detect/deobfuscate/unminify/codegen/sandbox) into a Pass.
// The existing modules stay in their current locations (src/detect, src/deobfuscate, etc.)
// and are called directly from here.

use crate::core::pass::{Pass, Detector, PassId};
use crate::core::artifact::{Artifact, OutputKind, Language};
use crate::core::detect::{DetectContext, DetectVerdict};

pub const PASS_ID: PassId = "js.deob";

// -- Detector: wraps crate::detect::detect --

#[derive(Debug)]
pub struct JsDetector;

impl Detector for JsDetector {
    fn id(&self) -> PassId { PASS_ID }

    fn detect(&self, ctx: &DetectContext) -> Option<DetectVerdict> {
        let src = ctx.as_str();
        let detection = crate::detect::detect(src);
        match detection.family {
            crate::detect::ObfuscatorFamily::Unknown => None,
            _ => Some(DetectVerdict::new(
                PASS_ID,
                detection.confidence,
                detection.markers,
                format!("detected: {:?}", detection.family),
            )),
        }
    }
}

pub static JS_DETECTOR: JsDetector = JsDetector;

// -- Pass: wraps crate::deobfuscate::deobfuscate --

#[derive(Debug)]
pub struct JsPass;

pub static JS_PASS: JsPass = JsPass;

impl Pass for JsPass {
    fn id(&self) -> PassId { PASS_ID }

    fn detector(&self) -> &'static dyn Detector { &JS_DETECTOR }

    fn output_kind(&self, _output: &Artifact) -> OutputKind {
        OutputKind::Source { language: Language::JavaScript, formatted: true }
    }

    fn run(&self, artifact: &Artifact) -> Result<Artifact, String> {
        let src = artifact.as_str();
        let opts = crate::deobfuscate::Options::default();
        let res = crate::deobfuscate::deobfuscate(src, &opts);
        Ok(Artifact::new_raw(res.code.into_bytes()))
    }
}
