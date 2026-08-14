// passes/js — JavaScript deobfuscation pass.
// Wraps existing jd JS logic (detect/deobfuscate/unminify/codegen/sandbox) into a Pass.
// Also includes esoteric decoder (jsfuck/jjencode/aaencode/packer) via boa sandbox.

pub mod esoteric;

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

// -- Pass: wraps crate::deobfuscate::deobfuscate + esoteric decoding --

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
        let detection = crate::detect::detect(src);

        // Try esoteric decoding first (jsfuck/jjencode/aaencode/packer)
        if matches!(detection.family,
            crate::detect::ObfuscatorFamily::DeanEdwardsPacker
            | crate::detect::ObfuscatorFamily::JsFuck
            | crate::detect::ObfuscatorFamily::JjEncode
            | crate::detect::ObfuscatorFamily::AaEncode
        ) {
            if let Some(decoded) = esoteric::decode(src, &detection, opts.timeout) {
                // Run the decoded code through deobfuscation + unminify
                let res = crate::deobfuscate::deobfuscate(&decoded, &opts);
                return Ok(Artifact::new_raw(res.code.into_bytes()));
            }
            // Esoteric decode failed — fall through to normal processing
        }

        // Normal deobfuscation pipeline (obfuscator.io / minified / etc.)
        let res = crate::deobfuscate::deobfuscate(src, &opts);
        Ok(Artifact::new_raw(res.code.into_bytes()))
    }
}
