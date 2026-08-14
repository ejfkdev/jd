// passes/wasm — WebAssembly module analysis and WAT disassembly.
use crate::core::pass::{Pass, Detector, PassId};
use crate::core::artifact::{Artifact, OutputKind, Language};
use crate::core::detect::{DetectContext, DetectVerdict};

pub const PASS_ID: PassId = "wasm.deob";
const WASM_MAGIC: [u8; 4] = [0x00, 0x61, 0x73, 0x6d];

#[derive(Debug)]
pub struct WasmDetector;
impl Detector for WasmDetector {
    fn id(&self) -> PassId { PASS_ID }
    fn detect(&self, ctx: &DetectContext) -> Option<DetectVerdict> {
        let b = ctx.bytes;
        if b.len() < 8 || &b[..4] != &WASM_MAGIC { return None; }
        if b[4] != 0x01 || b[5] != 0x00 { return None; }
        Some(DetectVerdict::new(PASS_ID, 0.95, vec!["wasm-magic".into()], "WebAssembly module".into()))
    }
}
pub static WASM_DETECTOR: WasmDetector = WasmDetector;

#[derive(Debug)]
pub struct WasmPass;
pub static WASM_PASS: WasmPass = WasmPass;

impl Pass for WasmPass {
    fn id(&self) -> PassId { PASS_ID }
    fn detector(&self) -> &'static dyn Detector { &WASM_DETECTOR }
    fn output_kind(&self, _: &Artifact) -> OutputKind { OutputKind::Source { language: Language::Wasm, formatted: true } }
    fn run(&self, artifact: &Artifact) -> Result<Artifact, String> {
        let out = disassemble_wasm(&artifact.bytes)?;
        Ok(Artifact::new_raw(out.into_bytes()))
    }
}

fn disassemble_wasm(bytes: &[u8]) -> Result<String, String> {
    let mut out = String::from("(module\n");
    let mut counts = (0usize, 0usize, 0usize, 0usize, 0usize, 0usize, 0usize, 0usize);
    // (types, funcs, imports, exports, mems, globals, tables, instrs)

    for payload in wasmparser::Parser::new(0).parse_all(bytes) {
        let payload = payload.map_err(|e| format!("wasm parse: {e}"))?;
        match payload {
            wasmparser::Payload::Version { .. } => {}
            wasmparser::Payload::TypeSection(r) => {
                for item in r.into_iter() { let _ = item; counts.0 += 1; }
            }
            wasmparser::Payload::ImportSection(r) => {
                for item in r.into_iter() {
                    let imp = item.map_err(|e| format!("import: {e}"))?;
                    counts.2 += 1;
                    let (module, name) = match imp {
                        wasmparser::Imports::Single(_, i) => (i.module.to_string(), i.name.to_string()),
                        wasmparser::Imports::Compact1 { module, .. } => (module.to_string(), "(multiple)".to_string()),
                        wasmparser::Imports::Compact2 { module, .. } => (module.to_string(), "(multiple)".to_string()),
                    };
                    out.push_str(&format!("  (import \"{}\" \"{}\" ...)\n", module, name));
                }
            }
            wasmparser::Payload::FunctionSection(r) => {
                for item in r.into_iter() { let _ = item; counts.1 += 1; }
            }
            wasmparser::Payload::ExportSection(r) => {
                for item in r.into_iter() {
                    let exp = item.map_err(|e| format!("export: {e}"))?;
                    counts.3 += 1;
                    out.push_str(&format!("  (export \"{}\" ...)\n", exp.name));
                }
            }
            wasmparser::Payload::CodeSectionEntry(body) => {
                let locals = body.get_locals_reader().map(|r| r.into_iter().count()).unwrap_or(0);
                let instrs = body.get_operators_reader().map(|r| r.into_iter().count()).unwrap_or(0);
                counts.7 += instrs;
                out.push_str(&format!("  ;; function: {} locals, {} instructions\n", locals, instrs));
            }
            wasmparser::Payload::MemorySection(r) => {
                for item in r.into_iter() { let _ = item; counts.4 += 1; }
            }
            wasmparser::Payload::GlobalSection(r) => {
                for item in r.into_iter() { let _ = item; counts.5 += 1; }
            }
            wasmparser::Payload::TableSection(r) => {
                for item in r.into_iter() { let _ = item; counts.6 += 1; }
            }
            wasmparser::Payload::CustomSection(cs) => {
                if cs.name() == "name" {
                    out.push_str("  ;; name section present\n");
                }
            }
            _ => {}
        }
    }

    out.push_str(&format!("  ;; summary: {} types, {} functions, {} imports, {} exports, {} memories, {} globals, {} tables, {} total instructions\n",
        counts.0, counts.1, counts.2, counts.3, counts.4, counts.5, counts.6, counts.7));
    out.push_str(")");
    Ok(out)
}
