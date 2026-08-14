// codegen — pretty-print JavaScript using oxc codegen.
use crate::deobfuscate::Options;

pub fn generate(src: &str, _opts: &Options, warnings: &mut Vec<String>) -> String {
    let allocator = oxc_allocator::Allocator::default();
    // Use "unambiguous" to auto-detect ESM vs script — supports both
    // import/export (module) and require/module.exports (script).
    let source_type = oxc_span::SourceType::unambiguous();

    let parser = oxc_parser::Parser::new(&allocator, src, source_type)
        .with_options(oxc_parser::ParseOptions {
            allow_return_outside_function: true,
            ..Default::default()
        });

    let ret = parser.parse();

    if ret.panicked || !ret.errors.is_empty() {
        warnings.push("parse error in codegen, source returned as-is".into());
        return src.to_string();
    }

    let codegen = oxc_codegen::Codegen::new();
    codegen.build(&ret.program).code
}
