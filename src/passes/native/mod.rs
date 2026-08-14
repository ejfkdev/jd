// passes/native — Native binary analysis (PE/ELF/Mach-O).
// Parses executable headers, extracts section info, detects packers (UPX etc.).
// Uses manual binary parsing (no goblin/object dependency — lightweight).
// Reference: disrobe-pass-native (simplified — structure extraction, not full decompilation).

use crate::core::pass::{Pass, Detector, PassId};
use crate::core::artifact::{Artifact, OutputKind, ChildArtifact, ChildHandle};
use crate::core::detect::{DetectContext, DetectVerdict};

pub const PASS_ID: PassId = "native.unpack";

const PE_MAGIC: [u8; 2] = [0x4d, 0x5a]; // MZ
const ELF_MAGIC: [u8; 4] = [0x7f, 0x45, 0x4c, 0x46]; // \x7fELF
const MACHO_MAGIC_BE: [u8; 4] = [0xfe, 0xed, 0xfa, 0xce];
const MACHO_MAGIC_LE: [u8; 4] = [0xcf, 0xfa, 0xed, 0xfe];
const MACHO_FAT_BE: [u8; 4] = [0xca, 0xfe, 0xba, 0xbe];

#[derive(Debug)]
pub struct NativeDetector;

impl Detector for NativeDetector {
    fn id(&self) -> PassId { PASS_ID }

    fn detect(&self, ctx: &DetectContext) -> Option<DetectVerdict> {
        let b = ctx.bytes;
        if b.len() < 4 { return None; }

        if &b[..2] == &PE_MAGIC {
            // Check for UPX in section names
            let upx = detect_upx_pe(b);
            let family = if upx { "packer-archive" } else { "native-format" };
            let markers = if upx { vec!["pe-mz".into(), "upx-packed".into()] } else { vec!["pe-mz".into()] };
            return Some(DetectVerdict::new(PASS_ID, 0.55, markers,
                if upx { "PE executable (UPX packed)".into() } else { "PE executable (Windows)".into() }));
        }
        if &b[..4] == &ELF_MAGIC {
            let upx = detect_upx_elf(b);
            return Some(DetectVerdict::new(PASS_ID, 0.55,
                vec!["elf-magic".into()],
                if upx { "ELF executable (UPX packed)".into() } else { "ELF executable (Linux/BSD)".into() }));
        }
        if &b[..4] == &MACHO_MAGIC_BE || &b[..4] == &MACHO_MAGIC_LE {
            return Some(DetectVerdict::new(PASS_ID, 0.55,
                vec!["macho-magic".into()],
                "Mach-O executable (macOS/iOS)".into()));
        }
        if &b[..4] == &MACHO_FAT_BE {
            return Some(DetectVerdict::new(PASS_ID, 0.55,
                vec!["macho-fat".into()],
                "Mach-O Fat binary (multi-arch)".into()));
        }
        None
    }
}

pub static NATIVE_DETECTOR: NativeDetector = NativeDetector;

#[derive(Debug)]
pub struct NativePass;
pub static NATIVE_PASS: NativePass = NativePass;

impl Pass for NativePass {
    fn id(&self) -> PassId { PASS_ID }
    fn detector(&self) -> &'static dyn Detector { &NATIVE_DETECTOR }
    fn output_kind(&self, _: &Artifact) -> OutputKind {
        OutputKind::Source { language: crate::core::artifact::Language::Other, formatted: true }
    }
    fn run(&self, artifact: &Artifact) -> Result<Artifact, String> {
        let b = &artifact.bytes;
        let out = if b.len() >= 2 && &b[..2] == &PE_MAGIC {
            analyze_pe(b)?
        } else if b.len() >= 4 && &b[..4] == &ELF_MAGIC {
            analyze_elf(b)?
        } else if b.len() >= 4 && (&b[..4] == &MACHO_MAGIC_BE || &b[..4] == &MACHO_MAGIC_LE) {
            analyze_macho(b)?
        } else {
            return Err("unknown native format".into());
        };
        Ok(Artifact::new_raw(out.into_bytes()))
    }
}

/// Detect UPX packing in a PE file by checking section names.
fn detect_upx_pe(bytes: &[u8]) -> bool {
    // UPX packs sections with names like UPX0, UPX1
    // Search for "UPX" in the first 4KB (section headers area)
    let search_end = bytes.len().min(4096);
    find_ascii(&bytes[..search_end], b"UPX").is_some()
}

/// Detect UPX packing in an ELF file.
fn detect_upx_elf(bytes: &[u8]) -> bool {
    // UPX in ELF modifies the program header — search for "UPX" in the first 4KB
    let search_end = bytes.len().min(4096);
    find_ascii(&bytes[..search_end], b"UPX").is_some()
}

/// Find an ASCII string in binary data.
fn find_ascii(haystack: &[u8], needle: &[u8]) -> Option<usize> {
    haystack.windows(needle.len()).position(|w| w == needle)
}

/// Analyze a PE executable: extract PE header, sections, imports, packer info.
fn analyze_pe(bytes: &[u8]) -> Result<String, String> {
    let mut out = String::from("// PE Executable Analysis\n");

    // DOS header (first 64 bytes)
    if bytes.len() < 64 {
        return Err("PE file too short".into());
    }

    // DOS magic: MZ (0x4D5A)
    out.push_str("// DOS Magic: MZ\n");

    // PE header offset at offset 0x3C (e_lfanew)
    let pe_offset = u32::from_le_bytes([bytes[0x3C], bytes[0x3D], bytes[0x3E], bytes[0x3F]]) as usize;
    out.push_str(&format!("// PE Header offset: 0x{:04x}\n", pe_offset));

    if pe_offset + 4 > bytes.len() {
        out.push_str("// (truncated — PE header beyond file end)\n");
        return Ok(out);
    }

    // PE signature: "PE\0\0"
    let pe_sig = &bytes[pe_offset..pe_offset+4];
    if pe_sig == b"PE\x00\x00" {
        out.push_str("// PE Signature: valid\n");
    } else {
        out.push_str(&format!("// PE Signature: invalid ({:?})\n", pe_sig));
    }

    // COFF header (after PE sig)
    if pe_offset + 24 <= bytes.len() {
        let machine = u16::from_le_bytes([bytes[pe_offset+4], bytes[pe_offset+5]]);
        let num_sections = u16::from_le_bytes([bytes[pe_offset+6], bytes[pe_offset+7]]);
        let machine_str = match machine {
            0x014c => "x86 (i386)",
            0x8664 => "x86_64 (AMD64)",
            0x01c0 => "ARM",
            0xaa64 => "ARM64",
            _ => "unknown",
        };
        out.push_str(&format!("// Machine: {} (0x{:04x})\n", machine_str, machine));
        out.push_str(&format!("// Sections: {}\n", num_sections));

        // Optional header starts at pe_offset + 24
        if pe_offset + 24 + 2 <= bytes.len() {
            let opt_magic = u16::from_le_bytes([bytes[pe_offset+24], bytes[pe_offset+25]]);
            let bits = if opt_magic == 0x10b { "32-bit (PE32)" }
                       else if opt_magic == 0x20b { "64-bit (PE32+)" }
                       else { "unknown" };
            out.push_str(&format!("// Optional Header: {} (magic 0x{:04x})\n", bits, opt_magic));
        }
    }

    // Detect packers
    if detect_upx_pe(bytes) {
        out.push_str("// Packer: UPX detected\n");
        out.push_str("// To unpack: use 'upx -d file.exe' (requires UPX installed)\n");
    }

    // Extract section names (simplified — search for ASCII strings in section table)
    out.push_str("\n// Sections:\n");
    if pe_offset + 40 <= bytes.len() {
        // Section table starts after optional header (simplified — actual offset varies)
        // Search for common section names
        for name in &[".text", ".data", ".rdata", ".rsrc", ".bss", "UPX0", "UPX1", ".textbss"] {
            if find_ascii(bytes, name.as_bytes()).is_some() {
                out.push_str(&format!("//   section: {}\n", name));
            }
        }
    }

    // Extract import hints (search for common DLL names)
    out.push_str("\n// Import hints:\n");
    for dll in &["kernel32.dll", "user32.dll", "ntdll.dll", "advapi32.dll",
                  "ws2_32.dll", "wininet.dll", "crypt32.dll", "ole32.dll",
                  "msvcrt.dll", "comctl32.dll", "gdi32.dll"] {
        if find_ascii(bytes, dll.as_bytes()).is_some() {
            out.push_str(&format!("//   imports from: {}\n", dll));
        }
    }

    Ok(out)
}

/// Analyze an ELF executable: extract header, sections, dynamic linking info.
fn analyze_elf(bytes: &[u8]) -> Result<String, String> {
    let mut out = String::from("// ELF Executable Analysis\n");

    if bytes.len() < 64 {
        return Err("ELF file too short".into());
    }

    // ELF magic: \x7fELF
    out.push_str("// ELF Magic: \\x7fELF\n");

    // EI_CLASS (byte 4): 1=32-bit, 2=64-bit
    let class = bytes[4];
    let bits = match class { 1 => "32-bit (ELF32)", 2 => "64-bit (ELF64)", _ => "unknown" };
    out.push_str(&format!("// Class: {}\n", bits));

    // EI_DATA (byte 5): 1=LE, 2=BE
    let endian = bytes[5];
    let endian_str = match endian { 1 => "Little Endian", 2 => "Big Endian", _ => "unknown" };
    out.push_str(&format!("// Endian: {}\n", endian_str));

    // e_type (bytes 16-17): executable type
    if bytes.len() >= 18 {
        let e_type = u16::from_le_bytes([bytes[16], bytes[17]]);
        let type_str = match e_type {
            1 => "relocatable (object file)",
            2 => "executable",
            3 => "shared library (PIE)",
            4 => "core dump",
            _ => "unknown",
        };
        out.push_str(&format!("// Type: {} (0x{:04x})\n", type_str, e_type));
    }

    // e_machine (bytes 18-19)
    if bytes.len() >= 20 {
        let machine = u16::from_le_bytes([bytes[18], bytes[19]]);
        let machine_str = match machine {
            0x03 => "x86 (i386)",
            0x3e => "x86_64 (AMD64)",
            0x28 => "ARM",
            0xb7 => "AArch64",
            0x16 => "SPARC",
            0x32 => "PPC64",
            _ => "unknown",
        };
        out.push_str(&format!("// Machine: {} (0x{:04x})\n", machine_str, machine));
    }

    // Detect packers
    if detect_upx_elf(bytes) {
        out.push_str("// Packer: UPX detected\n");
        out.push_str("// To unpack: use 'upx -d file.elf' (requires UPX installed)\n");
    }

    // Extract section hints
    out.push_str("\n// Sections (by name search):\n");
    for name in &[".text", ".data", ".rodata", ".bss", ".init", ".fini",
                  ".got", ".plt", ".dynamic", ".note", ".comment", ".symtab", ".strtab"] {
        if find_ascii(bytes, name.as_bytes()).is_some() {
            out.push_str(&format!("//   section: {}\n", name));
        }
    }

    // Extract dynamic linking hints
    out.push_str("\n// Dynamic library hints:\n");
    for lib in &["libc.so", "libpthread.so", "libdl.so", "libm.so", "librt.so",
                  "libcrypto.so", "libssl.so", "libz.so", "libcurl.so",
                  "ld-linux", "libgcc_s.so", "libstdc++.so"] {
        if find_ascii(bytes, lib.as_bytes()).is_some() {
            out.push_str(&format!("//   links: {}\n", lib));
        }
    }

    Ok(out)
}

/// Analyze a Mach-O executable: extract header, segments, architectures.
fn analyze_macho(bytes: &[u8]) -> Result<String, String> {
    let mut out = String::from("// Mach-O Executable Analysis\n");

    if bytes.len() < 32 {
        return Err("Mach-O file too short".into());
    }

    let magic = u32::from_be_bytes([bytes[0], bytes[1], bytes[2], bytes[3]]);
    let is_64 = magic == 0xfeedfacf;
    let bits = if is_64 { "64-bit" } else { "32-bit" };

    out.push_str(&format!("// Magic: 0x{:08x} ({})\n", magic, bits));

    // cputype (bytes 4-7 in BE, or at offset)
    let cputype = u32::from_be_bytes([bytes[4], bytes[5], bytes[6], bytes[7]]);
    let cpu_str = match cputype {
        0x07 => "x86 (i386)",
        0x01000007 => "x86_64",
        0x0c => "ARM",
        0x0100000c => "ARM64",
        _ => "unknown",
    };
    out.push_str(&format!("// CPU Type: {} (0x{:08x})\n", cpu_str, cputype));

    // filetype (bytes 12-15)
    let filetype = u32::from_be_bytes([bytes[12], bytes[13], bytes[14], bytes[15]]);
    let type_str = match filetype {
        1 => "object",
        2 => "executable",
        3 => "library",
        4 => "core dump",
        _ => "unknown",
    };
    out.push_str(&format!("// File Type: {} (0x{:04x})\n", type_str, filetype));

    // Extract segment hints
    out.push_str("\n// Segments (by name search):\n");
    for name in &["__TEXT", "__DATA", "__LINKEDIT", "__OBJC", "__IMPORT",
                  "__PRELOAD", "__UNICODE", "__CTF", "__DWARF"] {
        if find_ascii(bytes, name.as_bytes()).is_some() {
            out.push_str(&format!("//   segment: {}\n", name));
        }
    }

    // Extract framework hints
    out.push_str("\n// Framework hints:\n");
    for fw in &["Foundation", "AppKit", "CoreFoundation", "CoreData",
                 "Security", "CoreGraphics", "CoreText", "IOKit",
                 "System.B.dylib", "libc", "libobjc.A.dylib"] {
        if find_ascii(bytes, fw.as_bytes()).is_some() {
            out.push_str(&format!("//   links: {}\n", fw));
        }
    }

    Ok(out)
}
