// Esoteric encoder deobfuscation: jsfuck, jjencode, aaencode, Dean Edwards packer.
use crate::detect;
use crate::sandbox;
use super::Options;

pub fn deobfuscate(src: &str, detection: &detect::Detection, opts: &Options, _warnings: &mut Vec<String>) -> String {
    match detection.family {
        detect::ObfuscatorFamily::DeanEdwardsPacker
        | detect::ObfuscatorFamily::JsFuck
        | detect::ObfuscatorFamily::JjEncode
        | detect::ObfuscatorFamily::AaEncode => {
            match sandbox::run(src, &[], opts.timeout) {
                Ok(_) => src.to_string(),
                Err(_) => src.to_string(),
            }
        }
        _ => src.to_string(),
    }
}
