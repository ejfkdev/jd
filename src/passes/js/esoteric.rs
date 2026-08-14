// passes/js/esoteric — esoteric encoder deobfuscation.
// Uses boa_engine sandbox to evaluate and decode jsfuck/jjencode/aaencode/packer.
// Reference: disrobe-pass-js-deob/src/esoteric/.

use crate::sandbox;
use crate::detect::{Detection, ObfuscatorFamily};

/// Decode esoteric-encoded JavaScript by evaluating it in the sandbox.
pub fn decode(src: &str, detection: &Detection, timeout: std::time::Duration) -> Option<String> {
    match detection.family {
        ObfuscatorFamily::DeanEdwardsPacker => decode_packer(src, timeout),
        ObfuscatorFamily::JsFuck => decode_eval(src, timeout),
        ObfuscatorFamily::JjEncode => decode_eval(src, timeout),
        ObfuscatorFamily::AaEncode => decode_eval(src, timeout),
        _ => None,
    }
}

/// Dean Edwards packer: eval(function(p,a,c,k,e,d){...}('packed',base,count,dict,0,{}))
/// Decode by evaluating the packer function and capturing the unpacked code.
fn decode_packer(src: &str, timeout: std::time::Duration) -> Option<String> {
    // The packer evaluates to a string — we replace eval with a return.
    // Strategy: wrap in a function that returns the decoded string.
    let wrapped = format!("({})", src.replace("eval(", "return("));
    match sandbox::run(&wrapped, &[], timeout) {
        Ok(vals) => {
            if let Some(Some(v)) = vals.first() {
                if let Some(s) = sandbox::decode_string(v) {
                    return Some(s);
                }
            }
            None
        }
        Err(_) => None,
    }
}

/// Generic eval-based decoder: evaluate the code and capture the output.
/// Works for jsfuck (()[]!), jjencode (~$), aaencode (kaomoji).
fn decode_eval(src: &str, timeout: std::time::Duration) -> Option<String> {
    // These encodings produce code that when evaluated returns a string.
    // We wrap the input in a function that captures the result.
    // For jsfuck: the expression evaluates to a string.
    // For jjencode/aaencode: similar pattern.
    let wrapped = format!("return ({src})");
    match sandbox::run(&wrapped, &[], timeout) {
        Ok(vals) => {
            if let Some(Some(v)) = vals.first() {
                if let Some(s) = sandbox::decode_string(v) {
                    return Some(s);
                }
                // Try converting to string
                let mut ctx = boa_engine::Context::default();
                if let Ok(s) = v.to_string(&mut ctx) {
                    return Some(s.to_std_string_escaped());
                }
            }
            None
        }
        Err(_) => None,
    }
}
