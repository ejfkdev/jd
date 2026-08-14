// sandbox — boa_engine JS runtime for decoder execution.
// Reference: disrobe's sandbox_guard.rs with RuntimeLimits.

use std::time::Duration;
use boa_engine::{Context, JsValue, Source, JsString, js_string};

/// Run JavaScript code in a sandboxed boa_engine context.
/// Executes setup code, then evaluates each call expression and returns results.
pub fn run(setup: &str, calls: &[String], _timeout: Duration) -> Result<Vec<Option<JsValue>>, Box<dyn std::error::Error>> {
    let mut ctx = Context::default();

    // Note: runtime limits disabled — obfuscator.io rotators need many loop
    // iterations to complete push/shift rotation. Timeout is handled by the
    // caller (jd CLI sets a per-file timeout).

    // Execute setup code (string array, decoders, rotator, wrappers)
    match ctx.eval(Source::from_bytes(setup)) {
        Ok(_) => {},
        Err(e) => {
            // Debug: save setup to file
            let _ = std::fs::write("/tmp/jd_sandbox_failed_setup.js", setup);
            return Err(format!("sandbox setup failed: {e}").into());
        }
    }

    if calls.is_empty() {
        return Ok(Vec::new());
    }

    // Build batch expression: [call1, call2, ...]
    let batch = format!(
        "[{}]",
        calls.iter().map(|c| format!("({c})")).collect::<Vec<_>>().join(",")
    );

    let val = match ctx.eval(Source::from_bytes(&batch)) {
        Ok(v) => v,
        Err(e) => {
            let _ = std::fs::write("/tmp/jd_sandbox_failed_batch.js", &batch);
            return Err(format!("sandbox batch failed: {e}").into());
        }
    };

    // Extract array elements
    let mut result = Vec::new();
    if let Some(obj) = val.as_object() {
        // Get array length
        if let Ok(len_val) = obj.get(js_string!("length"), &mut ctx) {
            if let Some(len) = len_val.as_number() {
                for i in 0..(len as usize) {
                    match obj.get(i, &mut ctx) {
                        Ok(v) => result.push(Some(v)),
                        Err(_) => result.push(None),
                    }
                }
            }
        }
    } else {
        result.push(Some(val));
    }

    Ok(result)
}

/// Extract a Rust String from a JsValue, coercing if needed.
pub fn decode_string(val: &JsValue) -> Option<String> {
    if let Some(s) = val.as_string() {
        return Some(s.to_std_string_escaped());
    }
    None
}
