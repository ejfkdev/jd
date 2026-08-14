// unminify — readability transforms using oxc AST.
use crate::deobfuscate::Options;

pub fn unminify(src: &str, _opts: &Options, warnings: &mut Vec<String>) -> String {
    let allocator = oxc_allocator::Allocator::default();
    let source_type = oxc_span::SourceType::mjs();

    let parser = oxc_parser::Parser::new(&allocator, src, source_type)
        .with_options(oxc_parser::ParseOptions {
            allow_return_outside_function: true,
            ..Default::default()
        });

    let ret = parser.parse();

    if ret.panicked || !ret.errors.is_empty() {
        return format_source(src);
    }

    let codegen = oxc_codegen::Codegen::new();
    let mut output = codegen.build(&ret.program).code;
    output = apply_text_transforms(&output);
    output
}

fn apply_text_transforms(src: &str) -> String {
    let mut out = src.to_string();
    out = out.replace("!0 ", "true ").replace("!0;", "true;").replace("!0)", "true)");
    out = out.replace("!1 ", "false ").replace("!1;", "false;").replace("!1)", "false)");
    out = out.replace("void 0", "undefined");
    out = out.replace("!!true", "true");
    out = out.replace("!!false", "false");
    out
}

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
