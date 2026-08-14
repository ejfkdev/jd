// deobfuscate — top-level pipeline: detect → deobfuscate → unminify → generate

pub mod obfuscator_io;
pub mod jsconfuser;
pub mod esoteric;

use std::time::Duration;
use crate::detect;
use crate::unminify;
use crate::codegen;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SandboxMode { Auto, Only, Off }

#[derive(Debug, Clone)]
pub struct Options {
    pub deobfuscate: bool,
    pub unminify: bool,
    pub sandbox: SandboxMode,
    pub timeout: Duration,
    pub verbose: bool,
}

impl Default for Options {
    fn default() -> Self {
        Self {
            deobfuscate: true,
            unminify: true,
            sandbox: SandboxMode::Auto,
            timeout: Duration::from_secs(10),
            verbose: false,
        }
    }
}

#[derive(Debug)]
pub struct Result {
    pub code: String,
    pub warnings: Vec<String>,
}

/// Deobfuscate JavaScript source.
pub fn deobfuscate(src: &str, opts: &Options) -> Result {
    let mut warnings = Vec::new();
    let detection = detect::detect(src);

    if opts.verbose {
        eprintln!("jd: detected: {:?} (confidence {:.2})", detection.family, detection.confidence);
    }

    let code = if opts.deobfuscate {
        match detection.family {
            detect::ObfuscatorFamily::ObfuscatorIo => obfuscator_io::deobfuscate(src, opts, &mut warnings),
            detect::ObfuscatorFamily::JsConfuser => jsconfuser::deobfuscate(src, opts, &mut warnings),
            detect::ObfuscatorFamily::DeanEdwardsPacker
            | detect::ObfuscatorFamily::JsFuck
            | detect::ObfuscatorFamily::JjEncode
            | detect::ObfuscatorFamily::AaEncode => esoteric::deobfuscate(src, &detection, opts, &mut warnings),
            _ => src.to_string(),
        }
    } else {
        src.to_string()
    };

    let code = if opts.unminify { unminify::unminify(&code, opts, &mut warnings) } else { code };
    let output = codegen::generate(&code, opts, &mut warnings);

    Result { code: output, warnings }
}
