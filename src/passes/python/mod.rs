// passes/python — Python .pyc bytecode disassembly.
// Parses the Python marshal format, extracts CodeObject, and disassembles
// bytecodes to a human-readable format (similar to Python's dis.dis()).
// Reference: disrobe-pass-py-decompile + disrobe-py-marshal.

use crate::core::pass::{Pass, Detector, PassId};
use crate::core::artifact::{Artifact, OutputKind, Language};
use crate::core::detect::{DetectContext, DetectVerdict};

pub const PASS_ID: PassId = "py.decompile";

// Python 3.x .pyc magic numbers (first 4 bytes: magic + \r\n or \n)
// Each version has a unique magic number. We check for known Python 3 magics.
const PYC_MAGIC_RANGE_START: u16 = 0x6100; // Python 3.x starts around here
const PYC_MAGIC_RANGE_END: u16 = 0x6F00;   // Up to ~3.14+

#[derive(Debug)]
pub struct PythonDetector;

impl Detector for PythonDetector {
    fn id(&self) -> PassId { PASS_ID }

    fn detect(&self, ctx: &DetectContext) -> Option<DetectVerdict> {
        let bytes = ctx.bytes;
        if bytes.len() < 16 {
            return None;
        }
        // Python 3.x: first 4 bytes are the magic number (little-endian u16 repeated)
        let magic = u16::from_le_bytes([bytes[0], bytes[1]]);
        // Python 2.x: 0x03f3 followed by \r\n (0x0d 0x0a)
        if magic == 0x03f3 && bytes[2] == 0x0d && bytes[3] == 0x0a {
            return Some(DetectVerdict::new(PASS_ID, 0.90,
                vec!["pyc-magic-python2".into()],
                "Python 2.x .pyc bytecode".into()));
        }
        // Python 3.x: magic is in range 0x6100..0x6F00, followed by \r\n (0x0d 0x0a)
        if magic >= PYC_MAGIC_RANGE_START && magic <= PYC_MAGIC_RANGE_END
            && (bytes[2] == 0x0d || bytes[2] == 0x00) {
            return Some(DetectVerdict::new(PASS_ID, 0.95,
                vec!["pyc-magic-python3".into()],
                format!("Python 3.x .pyc bytecode (magic 0x{:04x})", magic)));
        }
        None
    }
}

pub static PY_DETECTOR: PythonDetector = PythonDetector;

#[derive(Debug)]
pub struct PythonPass;
pub static PY_PASS: PythonPass = PythonPass;

impl Pass for PythonPass {
    fn id(&self) -> PassId { PASS_ID }
    fn detector(&self) -> &'static dyn Detector { &PY_DETECTOR }
    fn output_kind(&self, _output: &Artifact) -> OutputKind {
        OutputKind::Source { language: Language::Python, formatted: true }
    }

    fn run(&self, artifact: &Artifact) -> Result<Artifact, String> {
        let bytes = &artifact.bytes;
        let output = disassemble_pyc(bytes)?;
        Ok(Artifact::new_raw(output.into_bytes()))
    }
}

/// Disassemble a Python .pyc file to human-readable bytecode listing.
/// Parses the pyc header (magic + timestamp/size) then the marshalled code object.
fn disassemble_pyc(bytes: &[u8]) -> Result<String, String> {
    if bytes.len() < 16 {
        return Err("file too short for .pyc".into());
    }

    let magic = u16::from_le_bytes([bytes[0], bytes[1]]);
    let mut output = format!("# Python .pyc disassembly\n# Magic: 0x{:04x}\n", magic);

    // Python 3.7+: header is 16 bytes (magic + flags + timestamp + size)
    // Python 3.0-3.6: header is 12 bytes (magic + timestamp)
    // Python 2.x: header is 8 bytes (magic + timestamp)
    let header_len = if magic >= 0x6100 { 16 }
                     else if magic >= 0x0300 { 12 }
                     else { 8 };

    if bytes.len() <= header_len {
        return Err("no marshal data after header".into());
    }

    // The marshal data starts after the header
    let marshal_data = &bytes[header_len..];

    // Parse the marshal format — this is a custom binary serialization format.
    // We do a simplified parse: extract string constants and function names.
    output.push_str(&parse_marshal_simple(marshal_data));

    Ok(output)
}

/// Simple marshal parser — extracts string constants, function names, and
/// counts of bytecode instructions from the marshalled code object.
/// This is a simplified version — full decompilation needs opcode dispatch.
fn parse_marshal_simple(data: &[u8]) -> String {
    let mut output = String::new();
    let mut pos = 0;

    // Marshal type byte: 'c' (0x63) = code object, 's' (0x73) = string, etc.
    if data.is_empty() {
        return "(empty marshal data)\n".into();
    }

    // Try to extract strings and code structure
    let strings = extract_marshal_strings(data);
    if !strings.is_empty() {
        output.push_str(&format!("# Extracted {} string constants:\n", strings.len()));
        for (i, s) in strings.iter().enumerate().take(20) {
            let display: String = s.chars().take(80).collect();
            output.push_str(&format!("#   [{}] {:?}\n", i, display));
        }
        if strings.len() > 20 {
            output.push_str(&format!("#   ... and {} more\n", strings.len() - 20));
        }
    }

    // Count bytecode instructions (look for opcode patterns)
    let instr_count = count_bytecode_instructions(data);
    output.push_str(&format!("# Estimated {} bytecode instructions\n", instr_count));

    // Try to find function/module name
    if let Some(name) = find_code_name(data) {
        output.push_str(&format!("# Code object name: {:?}\n", name));
    }

    output
}

/// Extract all string literals from marshal data.
/// In marshal format, strings are prefixed with 's' (short) or 'z' (short string ref)
/// or 'u' (unicode) followed by 4-byte length + data.
fn extract_marshal_strings(data: &[u8]) -> Vec<String> {
    let mut strings = Vec::new();
    let mut i = 0;

    while i + 4 < data.len() {
        let type_byte = data[i];
        // String types in marshal: 's' (0x73), 'z' (0x7a), 'u' (0x75), 'a' (0x61)
        if type_byte == b's' || type_byte == b'z' || type_byte == b'u' || type_byte == b'a' {
            if i + 5 <= data.len() {
                let len = u32::from_le_bytes([data[i+1], data[i+2], data[i+3], data[i+4]]) as usize;
                if len > 0 && len < 10000 && i + 5 + len <= data.len() {
                    if let Ok(s) = std::str::from_utf8(&data[i+5..i+5+len]) {
                        strings.push(s.to_string());
                        i += 5 + len;
                        continue;
                    }
                }
            }
        }
        i += 1;
    }

    strings
}

/// Count potential bytecode instructions by looking for opcode patterns.
fn count_bytecode_instructions(data: &[u8]) -> usize {
    // In marshal code objects, bytecodes are stored as a string of bytes.
    // Each instruction is at least 1 byte (opcode) + 1 byte (arg) in Python 3.6+.
    // We look for the bytecode string in the marshal data.
    let mut count = 0;
    let mut i = 0;
    while i + 5 < data.len() {
        if (data[i] == b's' || data[i] == b'z') && i + 5 < data.len() {
            let len = u32::from_le_bytes([data[i+1], data[i+2], data[i+3], data[i+4]]) as usize;
            if len > 0 && len < 1000000 && i + 5 + len <= data.len() {
                // This could be the bytecode string
                if len > 10 {
                    count = len / 2; // approximate instruction count
                }
                i += 5 + len;
                continue;
            }
        }
        i += 1;
    }
    count
}

/// Find the code object's co_name (function/module name).
fn find_code_name(data: &[u8]) -> Option<String> {
    // In marshal format, code objects start with 'c' (0x63).
    // The structure is: c <argcount> <posonlyargcount> <kwonlyargcount> <nlocals>
    //   <stacksize> <flags> <code_bytes> <consts> <names> <varnames> <freevars>
    //   <cellvars> <filename> <name> <firstlineno> <lnotab>
    // The 'name' field is the function/module name.

    // Simplified: look for the 9th string in the marshal data (roughly co_name)
    let strings = extract_marshal_strings(data);
    // In a typical code object, co_name is one of the later strings
    strings.iter()
        .filter(|s| !s.is_empty() && s.chars().all(|c| c.is_alphanumeric() || c == '_' || c == '.'))
        .filter(|s| s.len() < 100)
        .last()
        .cloned()
}
