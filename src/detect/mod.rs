// detect — identify which obfuscator produced the input.
// Supports: obfuscator.io, JS-Confuser, Jscrambler, esoteric encoders (jsfuck/jjencode/aaencode/packer), bundlers.

use regex::Regex;
use std::sync::LazyLock;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum ObfuscatorFamily {
    ObfuscatorIo,
    JsConfuser,
    Jscrambler,
    JsObfu,
    Webpack,
    Vite,
    Esbuild,
    Browserify,
    JsFuck,
    JjEncode,
    AaEncode,
    DeanEdwardsPacker,
    Minified,
    Unknown,
}

#[derive(Debug, Clone)]
pub struct Detection {
    pub family: ObfuscatorFamily,
    pub confidence: f32,
    pub markers: Vec<String>,
}

const HEAD_BYTES: usize = 4096;
const OBF_IO_MARKER: &str = "obfuscator.io";
const JSCRAMBLER_MARKER: &str = "jscrambler";

// JS-Confuser markers: state_sum (while+switch), rgf (array of functions), flatten
static JSCONFUSER_STATE_SUM_RE: LazyLock<Option<Regex>> = LazyLock::new(|| {
    Regex::new(r"(?ms)(?:while|switch)\s*\(\s*[A-Za-z_$][\w$]*(?:\s*\+\s*[A-Za-z_$][\w$]*){1,}\s*(?:!==|===|\))").ok()
});

// JsFuck: only ()[]!
static JSFUCK_RE: LazyLock<Option<Regex>> = LazyLock::new(|| {
    Regex::new(r"^[\s()\[\]!]+$").ok()
});

// JJEncode: ~$ pattern
static JJENCODE_RE: LazyLock<Option<Regex>> = LazyLock::new(|| {
    Regex::new(r"~\$~\|").ok()
});

// AAEncode: kaomoji
static AAENCODE_RE: LazyLock<Option<Regex>> = LazyLock::new(|| {
    Regex::new(r"ﾟωﾟ").ok()
});

// Dean Edwards packer: eval(function(p,a,c,k,e,d)
static PACKER_RE: LazyLock<Option<Regex>> = LazyLock::new(|| {
    Regex::new(r"eval\(\s*function\s*\(\s*p\s*,\s*a\s*,\s*c\s*,\s*k\s*,\s*e\s*,\s*d\s*\)").ok()
});

// webpack markers
const WEBPACK_RE_MARKER: &str = "__webpack_require__";
const WEBPACK_CHUNK_MARKER: &str = "webpackChunk";

// Vite markers
const VITE_MARKER: &str = "__vite__";
const VITE_DEPS: &str = "__vite__mapDeps";

/// Detect the obfuscator family from source code.
pub fn detect(source: &str) -> Detection {
    let head = if source.len() > HEAD_BYTES { &source[..HEAD_BYTES] } else { source };
    let mut markers = Vec::new();

    // obfuscator.io banner
    if head.contains(OBF_IO_MARKER) {
        markers.push("obfuscator-io-banner".into());
        return Detection { family: ObfuscatorFamily::ObfuscatorIo, confidence: 0.95, markers };
    }

    // Jscrambler
    if head.contains(JSCRAMBLER_MARKER) {
        markers.push("jscrambler-banner".into());
        return Detection { family: ObfuscatorFamily::Jscrambler, confidence: 0.9, markers };
    }

    // JS-Confuser: state_sum pattern
    if let Some(ref re) = *JSCONFUSER_STATE_SUM_RE {
        if re.is_match(source) {
            markers.push("jsconfuser-state-sum".into());
            return Detection { family: ObfuscatorFamily::JsConfuser, confidence: 0.7, markers };
        }
    }

    // Dean Edwards packer
    if let Some(ref re) = *PACKER_RE {
        if re.is_match(head) {
            markers.push("dean-edwards-packer".into());
            return Detection { family: ObfuscatorFamily::DeanEdwardsPacker, confidence: 0.9, markers };
        }
    }

    // AAEncode (kaomoji)
    if let Some(ref re) = *AAENCODE_RE {
        if re.is_match(head) {
            markers.push("aaencode-kaomoji".into());
            return Detection { family: ObfuscatorFamily::AaEncode, confidence: 0.9, markers };
        }
    }

    // JJEncode
    if let Some(ref re) = *JJENCODE_RE {
        if re.is_match(head) {
            markers.push("jjencode-signature".into());
            return Detection { family: ObfuscatorFamily::JjEncode, confidence: 0.9, markers };
        }
    }

    // JsFuck
    if let Some(ref re) = *JSFUCK_RE {
        let stripped = source.chars().filter(|c| !c.is_whitespace()).collect::<String>();
        if stripped.len() > 100 && re.is_match(&stripped) {
            markers.push("jsfuck-purity".into());
            return Detection { family: ObfuscatorFamily::JsFuck, confidence: 0.8, markers };
        }
    }

    // Bundlers
    if head.contains(WEBPACK_RE_MARKER) || head.contains(WEBPACK_CHUNK_MARKER) {
        markers.push("webpack-runtime".into());
        return Detection { family: ObfuscatorFamily::Webpack, confidence: 0.85, markers };
    }
    if head.contains(VITE_DEPS) {
        markers.push("vite-mapDeps".into());
        return Detection { family: ObfuscatorFamily::Vite, confidence: 0.85, markers };
    }

    // Fallback: minified (single long line)
    let line_count = source.lines().count();
    if line_count <= 5 && source.len() > 1000 {
        markers.push("minified-single-line".into());
        return Detection { family: ObfuscatorFamily::Minified, confidence: 0.5, markers };
    }

    // Unknown — try obfuscator.io detection via string array pattern
    if looks_like_obfuscator_io(source) {
        markers.push("string-array-pattern".into());
        return Detection { family: ObfuscatorFamily::ObfuscatorIo, confidence: 0.6, markers };
    }

    Detection { family: ObfuscatorFamily::Unknown, confidence: 0.0, markers }
}

/// Heuristic: does the source look like obfuscator.io output?
fn looks_like_obfuscator_io(source: &str) -> bool {
    // Look for string array function + decoder function patterns
    let has_hex_names = source.contains("_0x") || source.contains("_0x5");
    let has_string_array = source.contains("function ") && source.contains("return ") && source.contains("push(") && source.contains("shift(");
    has_hex_names && has_string_array
}
