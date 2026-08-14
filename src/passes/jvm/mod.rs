// passes/jvm — Java classfile parsing and disassembly.
use crate::core::pass::{Pass, Detector, PassId};
use crate::core::artifact::{Artifact, OutputKind, Language};
use crate::core::detect::{DetectContext, DetectVerdict};

pub const PASS_ID: PassId = "jvm.decompile";
const CLASS_MAGIC: [u8; 4] = [0xCA, 0xFE, 0xBA, 0xBE];
const DEX_MAGIC: [u8; 4] = [0x64, 0x65, 0x78, 0x0a];

#[derive(Debug)]
pub struct JvmDetector;
impl Detector for JvmDetector {
    fn id(&self) -> PassId { PASS_ID }
    fn detect(&self, ctx: &DetectContext) -> Option<DetectVerdict> {
        let b = ctx.bytes;
        if b.len() < 4 { return None; }
        if &b[..4] == &CLASS_MAGIC {
            return Some(DetectVerdict::new(PASS_ID, 0.95, vec!["class-cafebabe".into()], "JVM class file".into()));
        }
        if &b[..4] == &DEX_MAGIC {
            return Some(DetectVerdict::new(PASS_ID, 0.90, vec!["dex-magic".into()], "Android DEX file".into()));
        }
        None
    }
}
pub static JVM_DETECTOR: JvmDetector = JvmDetector;

#[derive(Debug)]
pub struct JvmPass;
pub static JVM_PASS: JvmPass = JvmPass;

impl Pass for JvmPass {
    fn id(&self) -> PassId { PASS_ID }
    fn detector(&self) -> &'static dyn Detector { &JVM_DETECTOR }
    fn output_kind(&self, _: &Artifact) -> OutputKind { OutputKind::Source { language: Language::Java, formatted: true } }
    fn run(&self, artifact: &Artifact) -> Result<Artifact, String> {
        let out = disassemble_class(&artifact.bytes)?;
        Ok(Artifact::new_raw(out.into_bytes()))
    }
}

fn disassemble_class(bytes: &[u8]) -> Result<String, String> {
    if bytes.len() < 10 || &bytes[..4] != &CLASS_MAGIC {
        return Err("not a valid class file".into());
    }
    let mut out = String::from("// JVM Class File Disassembly\n");
    let major = u16::from_be_bytes([bytes[6], bytes[7]]);
    let pool_count = u16::from_be_bytes([bytes[8], bytes[9]]);
    let version = match major { 52 => "Java 8", 55 => "Java 11", 61 => "Java 17", 65 => "Java 21", _ => "unknown" };
    out.push_str(&format!("// version: {} (major={})\n", version, major));
    out.push_str(&format!("// constant_pool_count: {}\n", pool_count));

    use std::io::Cursor;
    match classfile_parser::parse_class_from_reader(&mut Cursor::new(bytes), "class.bin".to_string()) {
        Ok(class) => {
            out.push_str(&format!("// access_flags: {:?}\n", class.access_flags));
            // Resolve class name from constant pool
            if (class.this_class as usize) > 0 && (class.this_class as usize) <= class.const_pool.len() {
                if let classfile_parser::constant_info::ConstantInfo::Class(ref c) = class.const_pool[class.this_class as usize - 1] {
                    if (c.name_index as usize) > 0 && (c.name_index as usize) <= class.const_pool.len() {
                        if let classfile_parser::constant_info::ConstantInfo::Utf8(ref u) = class.const_pool[c.name_index as usize - 1] {
                            out.insert_str(0, &format!("// Class: {}\n", u.utf8_string));
                        }
                    }
                }
            }
            // Resolve super class
            if (class.super_class as usize) > 0 && (class.super_class as usize) <= class.const_pool.len() {
                if let classfile_parser::constant_info::ConstantInfo::Class(ref c) = class.const_pool[class.super_class as usize - 1] {
                    if (c.name_index as usize) > 0 && (c.name_index as usize) <= class.const_pool.len() {
                        if let classfile_parser::constant_info::ConstantInfo::Utf8(ref u) = class.const_pool[c.name_index as usize - 1] {
                            out.push_str(&format!("// Super: {}\n", u.utf8_string));
                        }
                    }
                }
            }
            // Fields
            out.push_str(&format!("\n// Fields ({}):\n", class.fields.len()));
            for f in &class.fields {
                let name = resolve_utf8(&class.const_pool, f.name_index);
                let desc = resolve_utf8(&class.const_pool, f.descriptor_index);
                out.push_str(&format!("//   {:?} {} : {}\n", f.access_flags, name, desc));
            }
            // Methods
            out.push_str(&format!("\n// Methods ({}):\n", class.methods.len()));
            for m in &class.methods {
                let name = resolve_utf8(&class.const_pool, m.name_index);
                let desc = resolve_utf8(&class.const_pool, m.descriptor_index);
                out.push_str(&format!("//   {:?} {} {}\n", m.access_flags, name, desc));
            }
            out.push_str(&format!("\n// Constant pool: {} entries\n", class.const_pool.len()));
        }
        Err(e) => {
            out.push_str(&format!("// parse error: {}\n", e));
        }
    }
    out.push_str("\n// Full Java source decompilation requires CFR/Procyon/fernflower backend.\n");
    Ok(out)
}

fn resolve_utf8(pool: &[classfile_parser::constant_info::ConstantInfo], index: u16) -> String {
    let idx = index as usize;
    if idx > 0 && idx <= pool.len() {
        if let classfile_parser::constant_info::ConstantInfo::Utf8(ref u) = pool[idx - 1] {
            return u.utf8_string.clone();
        }
    }
    format!("#{}", index)
}
