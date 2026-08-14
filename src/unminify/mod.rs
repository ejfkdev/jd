// unminify — readability transforms using oxc AST.
use crate::deobfuscate::Options;

pub fn unminify(src: &str, _opts: &Options, _warnings: &mut Vec<String>) -> String {
    let allocator = oxc_allocator::Allocator::default();
    let source_type = oxc_span::SourceType::mjs();

    let parser = oxc_parser::Parser::new(&allocator, src, source_type)
        .with_options(oxc_parser::ParseOptions {
            allow_return_outside_function: true,
            ..Default::default()
        });

    let ret = parser.parse();

    if ret.panicked || !ret.errors.is_empty() {
        // Parse error — try source-level formatting
        return format_source(src);
    }

    // Use oxc codegen for pretty-printing
    let codegen = oxc_codegen::Codegen::new();
    let output = codegen.build(&ret.program).code;

    // Apply post-codegen text transforms
    apply_text_transforms(&output)
}

/// Apply text-level readability transforms (post-codegen).
fn apply_text_transforms(src: &str) -> String {
    let mut out = src.to_string();

    // !0 → true, !1 → false
    out = out.replace("!0 ", "true ").replace("!0;", "true;").replace("!0)", "true)");
    out = out.replace("!1 ", "false ").replace("!1;", "false;").replace("!1)", "false)");

    // void 0 → undefined
    out = out.replace("void 0", "undefined");

    // Remove double-not on literals
    out = out.replace("!!true", "true");
    out = out.replace("!!false", "false");

    // !false → true, !true → false
    out = out.replace("!false", "true");
    out = out.replace("!true", "false");

    // !![] → true, ![] → false (obfuscator.io boolean obfuscation)
    out = out.replace("!![]", "true");
    out = out.replace("![]", "false");

    // while (!false) → while (true)
    out = out.replace("while ( true )", "while (true)");

    // computed member access → dot access: obj["method"] → obj.method
    // This is a safe transform when the key is a valid identifier.
    out = replace_bracket_with_dot(&out);

    // Fix number-dot-method access: 1024.toFixed → (1024).toFixed
    out = fix_number_dot_access(&out);

    // Clean up leading semicolons from removed helper code
    while out.starts_with('\n') || out.starts_with(';') || out.starts_with(' ') {
        out = out.trim_start_matches('\n').trim_start_matches(';').trim_start_matches(' ').to_string();
        // Only trim once per leading char type
        break;
    }
    out = out.trim_start().to_string();

    out
}

/// Replace bracket notation obj["key"] with dot notation obj.key
/// when "key" is a valid identifier. Skips keyword prefixes (async, await, etc).
fn replace_bracket_with_dot(s: &str) -> String {
    let re = regex::Regex::new(r#"\b(\w+)\s*\["([a-zA-Z_$][a-zA-Z0-9_$]*)"\]"#).unwrap();
    let keywords = ["async", "await", "new", "typeof", "void", "delete", "in",
        "instanceof", "yield", "return", "throw", "do", "if", "else", "for",
        "while", "function", "var", "let", "const", "switch", "case", "default",
        "try", "catch", "finally", "class", "extends", "super", "import", "export",
        "this", "true", "false", "null", "undefined", "static", "get", "set"];
    re.replace_all(s, |caps: &regex::Captures| {
        let prev_word = &caps[1];
        let key = &caps[2];
        // Don't replace if preceding word is a keyword
        if keywords.contains(&prev_word) {
            return format!("{prev_word}[\"{key}\"]");
        }
        // Don't replace if key is a reserved word
        if keywords.contains(&key) {
            return format!("{prev_word}[\"{key}\"]");
        }
        format!("{prev_word}.{key}")
    }).to_string()
}

/// Fix number-dot-method access patterns (e.g. 1024.toFixed → (1024).toFixed).
fn fix_number_dot_access(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    let chars: Vec<char> = s.chars().collect();
    let mut i = 0;

    while i < chars.len() {
        // Skip string literals
        if chars[i] == '"' || chars[i] == '\'' {
            let quote = chars[i];
            out.push(chars[i]);
            i += 1;
            while i < chars.len() {
                if chars[i] == '\\' && i + 1 < chars.len() {
                    out.push(chars[i]); out.push(chars[i+1]); i += 2; continue;
                }
                out.push(chars[i]);
                if chars[i] == quote { i += 1; break; }
                i += 1;
            }
            continue;
        }

        // Skip comments
        if chars[i] == '/' && i + 1 < chars.len() && chars[i+1] == '/' {
            while i < chars.len() && chars[i] != '\n' { out.push(chars[i]); i += 1; }
            continue;
        }
        if chars[i] == '/' && i + 1 < chars.len() && chars[i+1] == '*' {
            out.push('/'); out.push('*'); i += 2;
            while i + 1 < chars.len() && !(chars[i] == '*' && chars[i+1] == '/') {
                out.push(chars[i]); i += 1;
            }
            if i + 1 < chars.len() { out.push('*'); out.push('/'); i += 2; }
            continue;
        }

        // Skip regex literals: /pattern/flags
        // Detect regex context: / after operator, bracket, comma, etc.
        if chars[i] == '/' && i + 1 < chars.len() && chars[i+1] != '/' && chars[i+1] != '*' {
            // Check if this is a regex (preceded by non-identifier)
            let is_regex = if out.is_empty() { true }
            else {
                let prev = out.chars().last().unwrap();
                matches!(prev, '(' | ',' | '=' | ':' | '[' | '!' | '&' | '|' | '<' | '>' |
                    '+' | '-' | '*' | '%' | '~' | '^' | '?' | '{' | ';' | '\n' | ' ' | '\t')
            };
            if is_regex {
                out.push('/'); i += 1;
                let mut in_class = false;
                while i < chars.len() {
                    if chars[i] == '\\' && i + 1 < chars.len() {
                        out.push(chars[i]); out.push(chars[i+1]); i += 2; continue;
                    }
                    if chars[i] == '[' { in_class = true; }
                    if chars[i] == ']' { in_class = false; }
                    out.push(chars[i]);
                    if chars[i] == '/' && !in_class {
                        i += 1;
                        // Skip flags
                        while i < chars.len() && chars[i].is_ascii_alphabetic() {
                            out.push(chars[i]); i += 1;
                        }
                        break;
                    }
                    i += 1;
                }
                continue;
            }
        }

        // Detect number followed by .identifier
        if chars[i].is_ascii_digit() || (chars[i] == '0' && i + 1 < chars.len() && (chars[i+1] == 'x' || chars[i+1] == 'X')) {
            // Check previous char — skip if part of identifier
            if !out.is_empty() {
                let prev = out.chars().last().unwrap();
                if prev.is_alphanumeric() || prev == '_' || prev == '$' {
                    out.push(chars[i]); i += 1; continue;
                }
            }
            // Consume number
            let start = i;
            if chars[i] == '0' && i + 1 < chars.len() && (chars[i+1] == 'x' || chars[i+1] == 'X') {
                i += 2;
                while i < chars.len() && chars[i].is_ascii_hexdigit() { i += 1; }
            } else {
                while i < chars.len() && chars[i].is_ascii_digit() { i += 1; }
                if i < chars.len() && chars[i] == '.' && i + 1 < chars.len() && chars[i+1].is_ascii_digit() {
                    i += 1;
                    while i < chars.len() && chars[i].is_ascii_digit() { i += 1; }
                }
            }
            // Check: number followed by .identifier
            if i < chars.len() && chars[i] == '.' && i + 1 < chars.len()
                && (chars[i+1].is_ascii_alphabetic() || chars[i+1] == '_' || chars[i+1] == '$')
            {
                out.push('(');
                for j in start..i { out.push(chars[j]); }
                out.push(')');
            } else {
                for j in start..i { out.push(chars[j]); }
            }
            continue;
        }

        out.push(chars[i]);
        i += 1;
    }
    out
}

/// Source-level formatting fallback (when oxc parse fails).
fn format_source(src: &str) -> String {
    let mut out = String::with_capacity(src.len() * 2);
    let mut in_string = false;
    let mut string_char = '\0';
    let chars: Vec<char> = src.chars().collect();
    let mut i = 0;

    while i < chars.len() {
        let c = chars[i];
        if in_string {
            if c == '\\' && i + 1 < chars.len() {
                out.push(c); out.push(chars[i + 1]); i += 2; continue;
            }
            if c == string_char { in_string = false; }
            out.push(c); i += 1; continue;
        }
        match c {
            '"' | '\'' => { in_string = true; string_char = c; out.push(c); }
            ';' => { out.push(';'); out.push('\n'); }
            _ => out.push(c),
        }
        i += 1;
    }
    out
}
