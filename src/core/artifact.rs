// Artifact, OutputKind, Language — data types for the pipeline.

#[derive(Debug, Clone)]
pub struct Artifact {
    pub bytes: Vec<u8>,
}

impl Artifact {
    pub fn new_raw(bytes: Vec<u8>) -> Self { Self { bytes } }
    pub fn as_str(&self) -> &str { std::str::from_utf8(&self.bytes).unwrap_or("") }
}

#[derive(Debug, Clone)]
pub enum OutputKind {
    Source { language: Language, formatted: bool },
    Bytes { format_tag: &'static str, family: &'static str },
    Mixed { children: Vec<ChildHandle> },
}

#[derive(Debug, Clone)]
pub struct ChildHandle { pub relative_path: String, pub hint: Option<String> }

/// Hint that stops re-queueing (terminal child — don't process further).
pub const TERMINAL_HINT: &str = "jd.terminal";

#[derive(Debug)]
pub struct ChildArtifact { pub handle: ChildHandle, pub bytes: Vec<u8> }

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Language {
    JavaScript, TypeScript, Python, Java, Go, Rust, C, Cpp, Ruby, Php, Lua, Wasm, Other,
}
