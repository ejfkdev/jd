// passes/native — Native binary detection (PE/ELF/Mach-O) (placeholder).
// Detects executable formats for future unpacking/decompilation.
// Reference: disrobe-pass-native chain_detector.

use crate::core::pass::{Pass, Detector, PassId};
use crate::core::artifact::{Artifact, OutputKind, Language};
use crate::core::detect::{DetectContext, DetectVerdict};

pub const PASS_ID: PassId = "native.unpack";

// PE: MZ (0x4d 0x5a)
const PE_MAGIC: [u8; 2] = [0x4d, 0x5a];
// ELF: 0x7f 0x45 0x4c 0x46
const ELF_MAGIC: [u8; 4] = [0x7f, 0x45, 0x4c, 0x46];
// Mach-O: 0xfe 0xed 0xfa 0xce or 0xcf 0xfa 0xed 0xfe
const MACHO_MAGIC_BE: [u8; 4] = [0xfe, 0xed, 0xfa, 0xce];
const MACHO_MAGIC_LE: [u8; 4] = [0xcf, 0xfa, 0xed, 0xfe];

#[derive(Debug)]
pub struct NativeDetector;

impl Detector for NativeDetector {
    fn id(&self) -> PassId { PASS_ID }

    fn detect(&self, ctx: &DetectContext) -> Option<DetectVerdict> {
        let bytes = ctx.bytes;
        if bytes.len() < 4 {
            return None;
        }
        if &bytes[..2] == &PE_MAGIC {
            return Some(DetectVerdict::new(PASS_ID, 0.55,
                vec!["pe-mz-header".into()],
                "PE executable (Windows)".into()));
        }
        if &bytes[..4] == &ELF_MAGIC {
            return Some(DetectVerdict::new(PASS_ID, 0.55,
                vec!["elf-magic".into()],
                "ELF executable (Linux/BSD)".into()));
        }
        if &bytes[..4] == &MACHO_MAGIC_BE || &bytes[..4] == &MACHO_MAGIC_LE {
            return Some(DetectVerdict::new(PASS_ID, 0.55,
                vec!["macho-magic".into()],
                "Mach-O executable (macOS/iOS)".into()));
        }
        None
    }
}

pub static NATIVE_DETECTOR: NativeDetector = NativeDetector;

#[derive(Debug)]
pub struct NativePass;
pub static NATIVE_PASS: NativePass = NativePass;

impl Pass for NativePass {
    fn id(&self) -> PassId { PASS_ID }
    fn detector(&self) -> &'static dyn Detector { &NATIVE_DETECTOR }
    fn output_kind(&self, _output: &Artifact) -> OutputKind {
        OutputKind::Bytes { format_tag: "native-binary", family: "native-format" }
    }
    fn run(&self, _artifact: &Artifact) -> Result<Artifact, String> {
        // TODO: implement UPX unpacking, section extraction, etc.
        Err("Native binary unpacking not yet implemented".into())
    }
}
