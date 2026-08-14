// detect — identify which obfuscator produced the input.
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

static JSCONFUSER_STATE_SUM_RE: LazyLock<Option<Regex>> = LazyLock::new(|| {
    Regex::new(r"(?ms)(?:while|switch)\s*\(\s*[A-Za-z_$][\w$]*(?:\s*\+\s*[A-Za-z_$][\w$]*){1,}\s*(?:!==|===|\))").ok()
});

static JSFUCK_RE: LazyLock<Option<Regex>> = LazyLock::new(|| {
    Regex::new(r"^[\s()\[\]!]+$").ok()
});

static JJENCODE_RE: LazyLock<Option<Regex>> = LazyLock::new(|| {
    Regex::new(r"~\$~\|").ok()
});

static AAENCODE_RE: LazyLock<Option<Regex>> = LazyLock::new(|| {
    Regex::new(r"ﾟωﾟ").ok()
});

static PACKER_RE: LazyLock<Option<Regex>> = LazyLock::new(|| {
    Regex::new(r"eval\(\s*function\s*\(\s*p\s*,\s*a\s*,\s*c\s*,\s*k\s*,\s*e\s*,\s*d\s*\)").ok()
});

// obfuscator.io patterns
static HEX_FUNC_RE: LazyLock<Option<Regex>> = LazyLock::new(|| {
    // function _0xXXX() or var _0xXXX = function()
    Regex::new(r"(?:function\s+_0x[0-9a-fA-F]+\s*\(|var\s+_0x[0-9a-fA-F]+\s*=\s*function\s*\(|const\s+_0x[0-9a-fA-F]+\s*=\s*function\s*\()").ok()
});

static STRING_ARRAY_RE: LazyLock<Option<Regex>> = LazyLock::new(|| {
    // String array getter: function _0xXXX() { ... return _0xXXX = function(){return arr}(); ... }
    // or: function _0xXXX() { ... _0xXXX = function(){return arr}; return _0xXXX(); ... }
    // Supports both . and bracket notation
    Regex::new(r"function\s+_0x[0-9a-fA-F]+\s*\(\s*\)\s*\{[^}]*_0x[0-9a-fA-F]+\s*=\s*function\s*\(\s*\)\s*\{[^}]*return\s+\w+").ok()
});

static PUSH_SHIFT_RE: LazyLock<Option<Regex>> = LazyLock::new(|| {
    // push(shift()) pattern — obfuscator.io rotator (supports both .push and ['push'])
    Regex::new(r"(?:\.push\s*\(\s*\w+\.shift\s*\(\s*\)\s*\)|\['push'\]\s*\(\s*\w+\['shift'\]\s*\(\s*\)\s*\))").ok()
});

const WEBPACK_RE_MARKER: &str = "__webpack_require__";
const WEBPACK_CHUNK_MARKER: &str = "webpackChunk";
const VITE_DEPS: &str = "__vite__mapDeps";

pub fn detect(source: &str) -> Detection {
    // Use floor_char_boundary to avoid panicking on multi-byte UTF-8 chars.
    let head = if source.len() > HEAD_BYTES {
        &source[..source.floor_char_boundary(HEAD_BYTES)]
    } else {
        source
    };
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

    // AAEncode
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

    // obfuscator.io detection (without banner) — pattern-based
    let has_hex_funcs = HEX_FUNC_RE.as_ref().map_or(false, |re| re.is_match(source));
    let has_string_array = STRING_ARRAY_RE.as_ref().map_or(false, |re| re.is_match(source));
    let has_push_shift = PUSH_SHIFT_RE.as_ref().map_or(false, |re| re.is_match(source));
    let has_hex_names = source.contains("_0x");

    if has_hex_funcs && (has_string_array || has_push_shift) {
        markers.push("obfuscator-io-pattern".into());
        if has_push_shift { markers.push("push-shift-rotator".into()); }
        if has_string_array { markers.push("string-array-getter".into()); }
        return Detection { family: ObfuscatorFamily::ObfuscatorIo, confidence: 0.8, markers };
    }

    // Fallback: hex names + push_shift OR while(!![])
    if has_hex_names && (has_push_shift || source.contains("while(!![])") || source.contains("while (!!!![])")) {
        markers.push("hex-names-rotator".into());
        return Detection { family: ObfuscatorFamily::ObfuscatorIo, confidence: 0.6, markers };
    }

    // Minified: single long line
    let line_count = source.lines().count();
    if line_count <= 5 && source.len() > 1000 {
        markers.push("minified-single-line".into());
        return Detection { family: ObfuscatorFamily::Minified, confidence: 0.5, markers };
    }

    Detection { family: ObfuscatorFamily::Unknown, confidence: 0.0, markers }
}
