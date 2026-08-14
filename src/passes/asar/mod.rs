// passes/asar — Electron asar archive unpacking.
// Asar format: 4-byte pickle header (size LE) + 4-byte padding + JSON header + concatenated file data.
// The JSON header is a tree of {files: {name: {files/offset/size/unpacked/link}}}.
// Reference: disrobe-pass-webview/src/electron.rs.

use std::collections::BTreeMap;
use serde::Deserialize;

use crate::core::pass::{Pass, Detector, PassId};
use crate::core::artifact::{Artifact, OutputKind, ChildArtifact, ChildHandle};
use crate::core::detect::{DetectContext, DetectVerdict};

pub const PASS_ID: PassId = "asar.unpack";

const ANCHOR: &[u8] = b"{\"files\":";

#[derive(Debug, Deserialize)]
struct AsarNode {
    #[serde(default)]
    files: Option<BTreeMap<String, AsarNode>>,
    #[serde(default)]
    offset: Option<String>,
    #[serde(default)]
    size: Option<u64>,
    #[serde(default)]
    unpacked: Option<bool>,
    #[serde(default)]
    link: Option<String>,
}

// -- Detector --

#[derive(Debug)]
pub struct AsarDetector;

impl Detector for AsarDetector {
    fn id(&self) -> PassId { PASS_ID }

    fn detect(&self, ctx: &DetectContext) -> Option<DetectVerdict> {
        // Look for the asar header: {"files": preceded by pickle prefix
        // The pickle header is: 4 bytes (header_size LE) + 4 bytes (0x00000004) + header_size bytes of JSON
        let bytes = ctx.bytes;
        if bytes.len() < 16 {
            return None;
        }
        // Fast check: look for {"files": anchor within the first 8KB
        let search_end = bytes.len().min(8192);
        if let Some(pos) = find_subslice(&bytes[..search_end], ANCHOR) {
            // Verify pickle prefix (16 bytes before anchor)
            if pos >= 16 {
                let base = pos - 16;
                let header_size = u32::from_le_bytes([
                    bytes[base], bytes[base+1], bytes[base+2], bytes[base+3]
                ]) as usize;
                // Validate header size is reasonable
                if header_size > 0 && header_size < 100_000_000 {
                    return Some(DetectVerdict::new(
                        PASS_ID, 0.90,
                        vec!["asar-pickle-header".into()],
                        "Electron asar archive".into(),
                    ));
                }
            }
        }
        None
    }
}

pub static ASAR_DETECTOR: AsarDetector = AsarDetector;

// -- Pass --

#[derive(Debug)]
pub struct AsarPass;

pub static ASAR_PASS: AsarPass = AsarPass;

impl Pass for AsarPass {
    fn id(&self) -> PassId { PASS_ID }
    fn detector(&self) -> &'static dyn Detector { &ASAR_DETECTOR }

    fn output_kind(&self, _output: &Artifact) -> OutputKind {
        // Fan-out: extract multiple children (files inside the asar)
        OutputKind::Mixed { children: Vec::new() }
    }

    fn run(&self, artifact: &Artifact) -> Result<Artifact, String> {
        // Extract children and return them as a concatenated listing
        let children = extract_asar(&artifact.bytes)?;
        let mut output = String::from("// ASAR Archive Extracted\n");
        output.push_str(&format!("// {} files extracted:\n", children.len()));
        for child in &children {
            output.push_str(&format!("// - {} ({} bytes)\n", child.handle.relative_path, child.bytes.len()));
            if let Some(ref hint) = child.handle.hint {
                if hint == "javascript" {
                    // Include JS file content
                    if let Ok(content) = std::str::from_utf8(&child.bytes) {
                        output.push_str(&format!("\n// --- {} ---\n{}\n", child.handle.relative_path, content));
                    }
                }
            }
        }
        Ok(Artifact::new_raw(output.into_bytes()))
    }

    fn extract_children(&self, input: &Artifact) -> Result<Vec<ChildArtifact>, String> {
        extract_asar(&input.bytes)
    }
}

/// Extract all files from an asar archive.
fn extract_asar(bytes: &[u8]) -> Result<Vec<ChildArtifact>, String> {
    // Find the asar header
    let search_end = bytes.len().min(8192);
    let anchor_pos = find_subslice(&bytes[..search_end], ANCHOR)
        .ok_or("asar header not found")?;

    if anchor_pos < 16 {
        return Err("asar header too close to start".into());
    }

    let base = anchor_pos - 16;
    let header_size = u32::from_le_bytes([
        bytes[base], bytes[base+1], bytes[base+2], bytes[base+3]
    ]) as usize;

    // The JSON header starts at base + 16 (after pickle prefix: 4 + 4 + 8 padding)
    // Actually asar format: 4 bytes (header size LE) + 4 bytes (payload size LE = 4) + 4 bytes padding + JSON
    // The anchor {"files": starts right after the pickle prefix
    let json_start = anchor_pos;
    // The header_size from the pickle is the total size after the first 4-byte
    // field, which includes: 4 (json_size_field) + 4 (padding) + 4 (padding) + JSON
    // So JSON length = header_size - 8 (but may have padding at the end)
    // We need to find the actual end of JSON by scanning for the closing brace.
    let json_max_end = (json_start + header_size).min(bytes.len());

    // Find the actual JSON end by looking for the matching closing brace
    let json_bytes = &bytes[json_start..json_max_end];
    let json_str = std::str::from_utf8(json_bytes)
        .map_err(|e| format!("asar JSON parse error: {e}"))?;

    // The JSON may have trailing padding or file data after it.
    // Find the actual end of JSON by scanning for the matching closing brace
    // at the top level.
    let json_actual_end = find_json_end(json_bytes)
        .ok_or("asar JSON: could not find end of JSON object")?;
    let json_trimmed = &json_str[..json_actual_end];
    let root: AsarNode = serde_json::from_str(json_trimmed)
        .map_err(|e| format!("asar JSON parse error: {e}"))?;

    // Data section starts after: JSON (padded to 4 bytes)
    let json_padded_len = (json_actual_end + 3) & !3; // align to 4 bytes
    let data_base = json_start + json_padded_len;

    let mut children = Vec::new();
    extract_node(&root, "", bytes, data_base, &mut children);
    Ok(children)
}

fn extract_node(node: &AsarNode, prefix: &str, bytes: &[u8], data_base: usize, out: &mut Vec<ChildArtifact>) {
    if let Some(ref files) = node.files {
        for (name, child) in files {
            let path = if prefix.is_empty() { name.clone() } else { format!("{prefix}/{name}") };
            extract_node(child, &path, bytes, data_base, out);
        }
    } else {
        // Leaf node — extract file content
        let file_name = prefix.rsplit('/').next().unwrap_or(prefix);
        if let (Some(offset_str), Some(size)) = (&node.offset, node.size) {
            if let Ok(offset) = offset_str.parse::<usize>() {
                let absolute = data_base + offset;
                let end = absolute + size as usize;
                if end <= bytes.len() {
                    let content = bytes[absolute..end].to_vec();
                    // Determine hint: JS files get "js" hint for further processing
                    let hint = if file_name.ends_with(".js") || file_name.ends_with(".mjs") || file_name.ends_with(".cjs") {
                        Some("javascript".to_string())
                    } else {
                        None
                    };
                    out.push(ChildArtifact {
                        handle: ChildHandle {
                            relative_path: prefix.to_string(),
                            hint,
                        },
                        bytes: content,
                    });
                }
            }
        }
        // Handle symlinks
        if let Some(ref link) = node.link {
            out.push(ChildArtifact {
                handle: ChildHandle {
                    relative_path: prefix.to_string(),
                    hint: Some("symlink".to_string()),
                },
                bytes: link.as_bytes().to_vec(),
            });
        }
    }
}

/// Find a byte slice within another, returns the starting index.
fn find_subslice(haystack: &[u8], needle: &[u8]) -> Option<usize> {
    haystack.windows(needle.len()).position(|w| w == needle)
}

/// Find the end position (exclusive) of a JSON object starting with `{`.
/// Scans for matching braces, respecting string literals.
fn find_json_end(bytes: &[u8]) -> Option<usize> {
    if bytes.is_empty() || bytes[0] != b'{' {
        return None;
    }
    let mut depth = 0i32;
    let mut in_string = false;
    let mut escaped = false;
    for (i, &b) in bytes.iter().enumerate() {
        if escaped {
            escaped = false;
            continue;
        }
        if in_string {
            if b == b'\\' { escaped = true; }
            else if b == b'"' { in_string = false; }
            continue;
        }
        match b {
            b'"' => in_string = true,
            b'{' => depth += 1,
            b'}' => {
                depth -= 1;
                if depth == 0 {
                    return Some(i + 1);
                }
            }
            _ => {}
        }
    }
    None
}
