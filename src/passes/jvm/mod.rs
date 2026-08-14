// passes/jvm — JVM classfile/jar detection (placeholder).
// Detects .class and .jar (zip) for future Java decompilation.
// Reference: disrobe-pass-jvm chain_detector.

use crate::core::pass::{Pass, Detector, PassId};
use crate::core::artifact::{Artifact, OutputKind, Language};
use crate::core::detect::{DetectContext, DetectVerdict};

pub const PASS_ID: PassId = "jvm.decompile";

// .class magic: 0xCA 0xFE 0xBA 0xBE
const CLASS_MAGIC: [u8; 4] = [0xCA, 0xFE, 0xBA, 0xBE];
// .jar (zip): 0x50 0x4B 0x03 0x04
const ZIP_MAGIC: [u8; 4] = [0x50, 0x4B, 0x03, 0x04];
// DEX: "dex\n" = 0x64 0x65 0x78 0x0a
const DEX_MAGIC: [u8; 4] = [0x64, 0x65, 0x78, 0x0a];

#[derive(Debug)]
pub struct JvmDetector;

impl Detector for JvmDetector {
    fn id(&self) -> PassId { PASS_ID }

    fn detect(&self, ctx: &DetectContext) -> Option<DetectVerdict> {
        let bytes = ctx.bytes;
        if bytes.len() < 4 {
            return None;
        }
        if &bytes[..4] == &CLASS_MAGIC {
            return Some(DetectVerdict::new(PASS_ID, 0.95,
                vec!["class-cafebabe".into()],
                "JVM class file".into()));
        }
        if &bytes[..4] == &DEX_MAGIC {
            return Some(DetectVerdict::new(PASS_ID, 0.90,
                vec!["dex-magic".into()],
                "Android DEX file".into()));
        }
        // JAR is a zip — check if it contains .class entries
        if &bytes[..4] == &ZIP_MAGIC {
            // Could be a JAR — needs deeper inspection
            return None; // Let other detectors handle plain zips
        }
        None
    }
}

pub static JVM_DETECTOR: JvmDetector = JvmDetector;

#[derive(Debug)]
pub struct JvmPass;
pub static JVM_PASS: JvmPass = JvmPass;

impl Pass for JvmPass {
    fn id(&self) -> PassId { PASS_ID }
    fn detector(&self) -> &'static dyn Detector { &JVM_DETECTOR }
    fn output_kind(&self, _output: &Artifact) -> OutputKind {
        OutputKind::Source { language: Language::Java, formatted: true }
    }
    fn run(&self, _artifact: &Artifact) -> Result<Artifact, String> {
        // TODO: implement JVM classfile decompilation (using CFR/Procyon backend)
        Err("JVM decompilation not yet implemented".into())
    }
}
