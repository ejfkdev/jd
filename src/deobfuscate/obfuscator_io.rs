// obfuscator.io deobfuscation: string-array, rotator, decoder detection + sandbox decode.

use crate::sandbox;
use super::Options;

pub fn deobfuscate(src: &str, opts: &Options, _warnings: &mut Vec<String>) -> String {
    let det = detect_string_array_and_decoders(src);
    if opts.verbose {
        if let Some(ref d) = det {
            eprintln!("jd: obfuscator.io: array_fn={}, decoders={:?}, wrappers={:?}, rotator={}",
                d.array_fn_name, d.decoder_names, d.wrapper_names, d.rotator_source.is_some());
        } else {
            eprintln!("jd: obfuscator.io: no string array detected");
        }
    }
    if det.is_none() {
        return src.to_string();
    }
    let det = det.unwrap();

    let setup = build_setup_code(src, &det);
    if opts.verbose {
        eprintln!("jd: setup len={}", setup.len());
        eprintln!("jd: has $QL in setup: {}", setup.contains("$QL"));
        // Save setup to file for debugging
        let _ = std::fs::write("/tmp/jd_setup_debug.js", &setup);
    }
    if setup.is_empty() {
        return src.to_string();
    }

    let call_sites = collect_call_sites(src, &det);
    if opts.verbose {
        eprintln!("jd: call_sites: {} found", call_sites.len());
        for (i, s) in call_sites.iter().enumerate().take(5) {
            eprintln!("jd:   [{}] {}", i, s);
        }
    }
    if call_sites.is_empty() {
        return remove_helpers(src, &det);
    }

    match sandbox::run(&setup, &call_sites, opts.timeout) {
        Ok(vals) => {
            if opts.verbose {
                eprintln!("jd: sandbox returned {} values", vals.len());
                for (i, v) in vals.iter().enumerate().take(5) {
                    if let Some(val) = v {
                        if let Some(s) = sandbox::decode_string(val) {
                            eprintln!("jd:   [{}] decoded: {:?}", i, s);
                        } else {
                            eprintln!("jd:   [{}] not a string: {:?}", i, val);
                        }
                    } else {
                        eprintln!("jd:   [{}] none", i);
                    }
                }
            }
            let mut output = src.to_string();
            for (i, call_src) in call_sites.iter().enumerate() {
                if let Some(Some(v)) = vals.get(i) {
                    if let Some(s) = sandbox::decode_string(v) {
                        let replacement = format!("\"{}\"", escape_js_string(&s));
                        output = output.replace(call_src, &replacement);
                    }
                }
            }
            let output = remove_helpers(&output, &det);
            if opts.verbose {
                eprintln!("jd: after remove_helpers, has _0x25ad: {}", output.contains("_0x25ad"));
            }
            output
        }
        Err(e) => {
            if opts.verbose {
                eprintln!("jd: sandbox error: {}", e);
            }
            src.to_string()
        }
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
    let mut escaped = false;
    let mut end = idx;

    for (i, c) in src[idx..].char_indices() {
        if escaped {
            escaped = false;
            continue;
        }
        if in_string {
            if c == '\\' { escaped = true; continue; }
            if c == string_char { in_string = false; }
            continue;
        }
        match c {
            '\\' => escaped = true,
            '"' | '\'' => { in_string = true; string_char = c; }
            '{' => brace_count += 1,
            '}' => {
                brace_count -= 1;
                if brace_count == 0 { end = idx + i + 1; break; }
            }
            _ => {}
        }
    }
    Some(src[idx..end].to_string())
}

fn find_rotator(src: &str, array_fn_name: &str) -> Option<String> {
    if !src.contains("while(!![])") && !src.contains("while (!![])") {
        return None;
    }
    let call_pattern = format!("{array_fn_name},");
    let call_idx = src.find(&call_pattern)?;
    let before = &src[..call_idx];
    let start = before.rfind("(function").or_else(|| before.rfind("!function"))?;

    // Extract the full IIFE: (function(){...})(args)
    // Use the IIFE's opening ( as the starting paren_count=1.
    let rot_src = &src[start..];
    let mut paren_count = 0i32;
    let mut in_string = false;
    let mut string_char = '\0';
    let mut escaped = false;
    let mut end = 0;

    for (i, c) in rot_src.char_indices() {
        if escaped {
            escaped = false;
            continue;
        }
        if in_string {
            if c == '\\' { escaped = true; continue; }
            if c == string_char { in_string = false; }
            continue;
        }
        match c {
            '\\' => { escaped = true; }
            '"' | '\'' => { in_string = true; string_char = c; }
            '(' => paren_count += 1,
            ')' => {
                paren_count -= 1;
                if paren_count == 0 {
                    end = i + 1;
                    break;
                }
            }
            ';' if paren_count == 0 => {
                end = i;
                break;
            }
            _ => {}
        }
    }

    if end == 0 { return None; }
    Some(rot_src[..end].to_string())
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
    // Order: array → decoder → wrappers → rotator (rotator must be last, it's an
    // IIFE that executes immediately and calls the decoder)
    parts.push(det.array_fn_source.clone());
    for s in &det.decoder_sources { parts.push(s.clone()); }
    for (alias, target) in &det.wrapper_names { parts.push(format!("var {}={}", alias, target)); }
    if let Some(ref rot) = det.rotator_source {
        if rot.starts_with("function") { parts.push(format!("({})", rot)); }
        else { parts.push(rot.clone()); }
    }
    parts.join(";")
}

fn collect_call_sites(src: &str, det: &StringArrayDetection) -> Vec<String> {
    let mut sites = Vec::new();
    let mut names = det.decoder_names.clone();
    for (alias, _) in &det.wrapper_names { names.push(alias.clone()); }

    for name in &names {
        // Find call sites by scanning for name( and then balancing parens
        // while respecting string literals.
        let pattern = format!("{name}(");
        let mut search_from = 0;
        while let Some(rel) = src[search_from..].find(&pattern) {
            let start = search_from + rel;
            // Balance parens from start, respecting strings
            let call = extract_balanced_call(&src[start..]);
            if let Some(ref call) = call {
                if args_are_literal(call) {
                    sites.push(call.clone());
                }
                search_from = start + call.len();
            } else {
                search_from = start + pattern.len();
            }
        }
    }
    sites
}

/// Extract a balanced call expression from the start of s (s starts with name( ).
fn extract_balanced_call(s: &str) -> Option<String> {
    let chars: Vec<char> = s.chars().collect();
    if chars.is_empty() || chars[0] != '_' && !chars[0].is_alphabetic() {
        return None;
    }
    // Find the opening (
    let mut i = 0;
    while i < chars.len() && chars[i] != '(' { i += 1; }
    if i >= chars.len() { return None; }
    i += 1; // skip (
    let mut paren = 1i32;
    let mut in_str = false;
    let mut sc = '\0';
    let mut esc = false;
    let mut end = i;
    while i < chars.len() {
        let c = chars[i];
        if esc { esc = false; i += 1; continue; }
        if in_str {
            if c == '\\' { esc = true; i += 1; continue; }
            if c == sc { in_str = false; }
            i += 1; continue;
        }
        match c {
            '\\' => esc = true,
            '"' | '\'' => { in_str = true; sc = c; }
            '(' => paren += 1,
            ')' => { paren -= 1; if paren == 0 { end = i + 1; break; } }
            _ => {}
        }
        i += 1;
    }
    if end == 0 { return None; }
    Some(chars[..end].iter().collect())
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

fn remove_helpers(output: &str, det: &StringArrayDetection) -> String {
    let mut result = output.to_string();
    // Use the original function sources (from detection) to remove helpers.
    result = result.replace(&det.array_fn_source, "");
    for s in &det.decoder_sources {
        result = result.replace(s, "");
    }
    // Remove wrapper alias declarations
    for (alias, target) in &det.wrapper_names {
        // Match: const/var/let alias = target;
        let patterns = [
            format!("var {alias}={target};"),
            format!("var {alias}={target},"),
            format!("const {alias}={target};"),
            format!("const {alias}={target},"),
            format!("let {alias}={target};"),
            format!("let {alias}={target},"),
            format!("var {alias} = {target};"),
            format!("const {alias} = {target};"),
        ];
        for p in &patterns {
            result = result.replace(p, "");
        }
    }
    // Remove rotator IIFE
    if let Some(ref rot) = det.rotator_source {
        result = result.replace(rot, "");
    }
    result
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
