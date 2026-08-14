// sandbox — boa_engine JS runtime for decoder execution.
use std::time::Duration;
use boa_engine::{Context, JsValue, Source, js_string};

pub fn run(setup: &str, calls: &[String], _timeout: Duration) -> Result<Vec<Option<JsValue>>, Box<dyn std::error::Error>> {
    let mut ctx = Context::default();

    let _ = ctx.eval(Source::from_bytes(setup))?;

    if calls.is_empty() {
        return Ok(Vec::new());
    }

    let batch = format!("[{}]", calls.iter().map(|c| format!("({c})")).collect::<Vec<_>>().join(","));

    match ctx.eval(Source::from_bytes(&batch)) {
        Ok(val) => {
            let mut result = Vec::new();
            if let Some(obj) = val.as_object() {
                if let Ok(len_val) = obj.get(js_string!("length"), &mut ctx) {
                    if let Some(len) = len_val.as_number() {
                        for i in 0..(len as usize) {
                            if let Ok(v) = obj.get(i, &mut ctx) {
                                result.push(Some(v));
                            } else {
                                result.push(None);
                            }
                        }
                    }
                }
            } else {
                result.push(Some(val));
            }
            Ok(result)
        }
        Err(e) => Err(format!("sandbox execution failed: {e}").into()),
    }
}

pub fn decode_string(val: &JsValue) -> Option<String> {
    let s = val.as_string()?;
    Some(s.to_std_string_escaped())
}
