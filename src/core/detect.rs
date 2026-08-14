// DetectContext, DetectVerdict — detection types.
use super::pass::PassId;

pub struct DetectContext<'a> { pub bytes: &'a [u8], pub path_hint: Option<&'a str>, pub depth: u8 }
impl<'a> DetectContext<'a> {
    pub fn as_str(&self) -> &str { std::str::from_utf8(self.bytes).unwrap_or("") }
}

#[derive(Debug, Clone)]
pub struct DetectVerdict {
    pub pass_id: PassId,
    pub confidence: f32,
    pub markers: Vec<String>,
    pub explain: String,
}

impl DetectVerdict {
    pub fn new(pass_id: PassId, confidence: f32, markers: Vec<String>, explain: String) -> Self {
        Self { pass_id, confidence, markers, explain }
    }
}
