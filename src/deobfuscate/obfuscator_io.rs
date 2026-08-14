// obfuscator.io deobfuscation: string-array, rotator, decoder detection + sandbox decode.

use crate::sandbox;
use super::Options;

pub fn deobfuscate(src: &str, opts: &Options, _warnings: &mut Vec<String>) -> String {
    let det = detect_string_array_and_decoders(src);
    if det.is_none() {
        return src.to_string();
    }
    let det = det.unwrap();

    let setup = build_setup_code(src, &det);
    if setup.is_empty() {
        return src.to_string();
    }

    let call_sites = collect_call_sites(src, &det);
    if call_sites.is_empty() {
        return remove_helpers(src, &det);
    }

    match sandbox::run(&setup, &call_sites, opts.timeout) {
        Ok(vals) => {
            let mut output = src.to_string();
            for (i, call_src) in call_sites.iter().enumerate() {
                if let Some(Some(v)) = vals.get(i) {
                    if let Some(s) = sandbox::decode_string(v) {
                        let replacement = format!("\"{}\"", escape_js_string(&s));
                        output = output.replace(call_src, &replacement);
                    }
                }
            }
            remove_helpers(&output, &det)
        }
        Err(_) => src.to_string(),
    }
}

pub struct StringArrayDetection {
    pub array_fn_name: String,
    pub array_fn_source: String,
    pub decoder_names: Vec<String>,
    pub decoder_sources: Vec<String>,
    pub rotator_source: Option<String>,
    pub wrapper_names: Vec<(String, String)>,
}

fn detect_string_array_and_decoders(src: &str) -> Option<StringArrayDetection> {
    let re = regex::Regex::new(r"function\s+(_0x[0-9a-fA-F]+)\s*\(\s*\)").ok()?;
    let caps = re.captures(src)?;
    let array_fn_name = caps.get(1)?.as_str().to_string();

    let dec_pattern = format!(r"function\s+(_0x[0-9a-fA-F]+)\s*\(\s*\w+\s*,?\s*\w*\s*\)[^;]*{}\(\)", array_fn_name);
    let dec_re = regex::Regex::new(&dec_pattern).ok()?;
    let dec_caps = dec_re.captures(src)?;
    let decoder_name = dec_caps.get(1)?.as_str().to_string();

    let array_fn_source = extract_function_source(src, &array_fn_name)?;
    let decoder_source = extract_function_source(src, &decoder_name)?;
    let rotator_source = find_rotator(src, &array_fn_name);
    let wrapper_names = find_wrappers(src, &decoder_name);

    Some(StringArrayDetection {
        array_fn_name,
        array_fn_source,
        decoder_names: vec![decoder_name],
        decoder_sources: vec![decoder_source],
        rotator_source,
        wrapper_names,
    })
}

fn extract_function_source(src: &str, name: &str) -> Option<String> {
    let pattern = format!("function {name}");
    let idx = src.find(&pattern)?;
    let mut brace_count = 0i32;
    let mut in_string = false;
    let mut string_char = '\0';
    let mut end = idx;

    for (i, c) in src[idx..].char_indices() {
        if in_string {
            if c == '\\' { continue; }
            if c == string_char { in_string = false; }
        } else {
            match c {
                '"' | '\'' => { in_string = true; string_char = c; }
                '{' => brace_count += 1,
                '}' => {
                    brace_count -= 1;
                    if brace_count == 0 { end = idx + i + 1; break; }
                }
                _ => {}
            }
        }
    }
    Some(src[idx..end].to_string())
}

fn find_rotator(src: &str, _array_fn_name: &str) -> Option<String> {
    if let Some(idx) = src.find("while(!![])") {
        let start = src[..idx].rfind("(function").or_else(|| src[..idx].rfind("!function"))?;
        Some(src[start..idx + 100].to_string())
    } else { None }
}

fn find_wrappers(src: &str, decoder_name: &str) -> Vec<(String, String)> {
    let mut wrappers = Vec::new();
    let pattern = format!(r"(?:const|var|let)\s+(_?[\w$]+)\s*=\s*{}\s*[;,]", decoder_name);
    if let Ok(re) = regex::Regex::new(&pattern) {
        for caps in re.captures_iter(src) {
            wrappers.push((caps.get(1).unwrap().as_str().to_string(), decoder_name.to_string()));
        }
    }
    wrappers
}

fn build_setup_code(_src: &str, det: &StringArrayDetection) -> String {
    let mut parts = Vec::new();
    parts.push(det.array_fn_source.clone());
    for s in &det.decoder_sources { parts.push(s.clone()); }
    if let Some(ref rot) = det.rotator_source {
        if rot.starts_with("function") { parts.push(format!("({})", rot)); }
        else { parts.push(rot.clone()); }
    }
    for (alias, target) in &det.wrapper_names { parts.push(format!("var {}={}", alias, target)); }
    parts.join(";")
}

fn collect_call_sites(src: &str, det: &StringArrayDetection) -> Vec<String> {
    let mut sites = Vec::new();
    let mut names = det.decoder_names.clone();
    for (alias, _) in &det.wrapper_names { names.push(alias.clone()); }
    for name in &names {
        let pattern = format!(r"\b{}\s*\(\s*[^;)]+\s*\)", name);
        if let Ok(re) = regex::Regex::new(&pattern) {
            for m in re.find_iter(src) {
                let call = m.as_str().to_string();
                if args_are_literal(&call) { sites.push(call); }
            }
        }
    }
    sites
}

fn args_are_literal(call: &str) -> bool {
    let inner = call.split_once('(').and_then(|(_, rest)| rest.rsplit_once(')'));
    match inner {
        Some((args, _)) => {
            for arg in args.split(',') {
                let arg = arg.trim();
                if arg.is_empty() { continue; }
                let ok = arg.parse::<f64>().is_ok()
                    || (arg.starts_with('\'') && arg.ends_with('\''))
                    || (arg.starts_with('"') && arg.ends_with('"'))
                    || (arg.starts_with("0x") && u64::from_str_radix(&arg[2..], 16).is_ok());
                if !ok { return false; }
            }
            true
        }
        None => false,
    }
}

fn remove_helpers(src: &str, det: &StringArrayDetection) -> String {
    let mut output = src.to_string();
    if let Some(func_src) = extract_function_source(src, &det.array_fn_name) {
        output = output.replace(&func_src, "");
    }
    for name in &det.decoder_names {
        if let Some(func_src) = extract_function_source(src, name) {
            output = output.replace(&func_src, "");
        }
    }
    output
}

fn escape_js_string(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for c in s.chars() {
        match c {
            '\\' => out.push_str("\\\\"),
            '"' => out.push_str("\\\""),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if (c as u32) < 0x20 => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out
}
