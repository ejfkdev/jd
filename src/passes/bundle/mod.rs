// passes/bundle — webpack/vite/rollup bundle unpacking.
// Detects bundler patterns and extracts individual modules.
// Reference: disrobe-pass-js-deob/src/bundle/ (simplified).

use crate::core::pass::{Pass, Detector, PassId};
use crate::core::artifact::{Artifact, OutputKind, ChildArtifact, ChildHandle};
use crate::core::detect::{DetectContext, DetectVerdict};

pub const PASS_ID: PassId = "js.unbundle";

// Bundler markers
const WEBPACK_REQUIRE: &str = "__webpack_require__";
const WEBPACK_CHUNK: &str = "webpackChunk";
const VITE_MAP_DEPS: &str = "__vite__mapDeps";
const VITE_IMPORT: &str = "import{";
const ROLLUP_BANNER: &str = "Rollup";

#[derive(Debug)]
pub struct BundleDetector;

impl Detector for BundleDetector {
    fn id(&self) -> PassId { PASS_ID }

    fn detect(&self, ctx: &DetectContext) -> Option<DetectVerdict> {
        let src = ctx.as_str();
        let head = if src.len() > 8192 { &src[..src.floor_char_boundary(8192)] } else { src };

        if head.contains(WEBPACK_REQUIRE) || head.contains(WEBPACK_CHUNK) {
            return Some(DetectVerdict::new(PASS_ID, 0.85,
                vec!["webpack-runtime".into()],
                "Webpack bundle".into()));
        }
        if head.contains(VITE_MAP_DEPS) {
            return Some(DetectVerdict::new(PASS_ID, 0.85,
                vec!["vite-mapDeps".into()],
                "Vite bundle".into()));
        }
        // Vite/Rollup ES module bundles: import{} from "./..."
        if head.starts_with(VITE_IMPORT) || (head.contains("import ") && head.contains("from\".")) {
            // Only flag as bundle if it's a single long line (minified bundle)
            let line_count = src.lines().count();
            if line_count <= 5 && src.len() > 5000 {
                return Some(DetectVerdict::new(PASS_ID, 0.60,
                    vec!["es-module-bundle".into()],
                    "ES module bundle (Vite/Rollup)".into()));
            }
        }
        None
    }
}

pub static BUNDLE_DETECTOR: BundleDetector = BundleDetector;

#[derive(Debug)]
pub struct BundlePass;

pub static BUNDLE_PASS: BundlePass = BundlePass;

impl Pass for BundlePass {
    fn id(&self) -> PassId { PASS_ID }
    fn detector(&self) -> &'static dyn Detector { &BUNDLE_DETECTOR }

    fn output_kind(&self, _output: &Artifact) -> OutputKind {
        // Fan-out: extract individual modules as children
        OutputKind::Mixed { children: Vec::new() }
    }

    fn run(&self, artifact: &Artifact) -> Result<Artifact, String> {
        // Extraction happens in extract_children
        Ok(artifact.clone())
    }

    fn extract_children(&self, input: &Artifact) -> Result<Vec<ChildArtifact>, String> {
        let src = input.as_str();
        let mut children = Vec::new();

        // Try webpack module extraction
        if src.contains(WEBPACK_REQUIRE) {
            children.extend(extract_webpack_modules(src));
        }

        // If no modules found, try generic function-module extraction
        if children.is_empty() {
            children.extend(extract_function_modules(src));
        }

        Ok(children)
    }
}

/// Extract webpack modules from a webpack bundle.
/// Webpack 4: (function(modules){...})([function(){...}, ...])
/// Webpack 5: var __webpack_modules__ = {0: function(){...}, ...}
fn extract_webpack_modules(src: &str) -> Vec<ChildArtifact> {
    let mut modules = Vec::new();

    // Look for webpack 5 pattern: var __webpack_modules__ = {...}
    if let Some(idx) = src.find("__webpack_modules__") {
        // Find the modules object — simplified extraction
        // In practice this needs proper AST parsing
        // For now, just split on the pattern
        let _ = idx;
    }

    // Look for webpack 4 IIFE pattern: (function(modules){...})([...])
    // The array of modules contains function() {} entries
    // This is a simplified heuristic — proper extraction needs AST
    modules
}

/// Generic function-module extraction: find function(){} patterns that look like modules.
fn extract_function_modules(src: &str) -> Vec<ChildArtifact> {
    // For Vite/Rollup bundles, modules are often concatenated with import/export statements.
    // We can't easily split them without AST analysis.
    // Return the whole file as a single child — it will be processed by the JS deobfuscator.
    vec![ChildArtifact {
        handle: ChildHandle {
            relative_path: "bundle.js".into(),
            hint: Some("javascript".into()),
        },
        bytes: src.as_bytes().to_vec(),
    }]
}
