// passes/bundle — webpack/vite/rollup bundle unpacking.
// Uses oxc parser to split bundled modules into individual files.
// Reference: disrobe-pass-js-deob/src/bundle/ (simplified).

use crate::core::pass::{Pass, Detector, PassId};
use crate::core::artifact::{Artifact, OutputKind, ChildArtifact, ChildHandle};
use crate::core::detect::{DetectContext, DetectVerdict};

pub const PASS_ID: PassId = "js.unbundle";

const WEBPACK_REQUIRE: &str = "__webpack_require__";
const WEBPACK_CHUNK: &str = "webpackChunk";
const VITE_MAP_DEPS: &str = "__vite__mapDeps";

#[derive(Debug)]
pub struct BundleDetector;

impl Detector for BundleDetector {
    fn id(&self) -> PassId { PASS_ID }

    fn detect(&self, ctx: &DetectContext) -> Option<DetectVerdict> {
        let src = ctx.as_str();
        let head_end = src.len().min(8192);
        let head = if head_end < src.len() {
            &src[..src.floor_char_boundary(head_end)]
        } else {
            src
        };

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
        // ES module bundles: import{} from "./..."
        if (head.starts_with("import{") || head.starts_with("import {")) && head.contains("from\".") {
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
        OutputKind::Mixed { children: Vec::new() }
    }

    fn run(&self, artifact: &Artifact) -> Result<Artifact, String> {
        Ok(artifact.clone())
    }

    fn extract_children(&self, input: &Artifact) -> Result<Vec<ChildArtifact>, String> {
        let src = input.as_str();
        let mut children = Vec::new();

        if src.contains(WEBPACK_REQUIRE) {
            children.extend(extract_webpack_modules(src));
        }

        // If no modules extracted, pass the whole bundle as one JS file
        // (it will be processed by the JS deobfuscator pass)
        if children.is_empty() {
            children.push(ChildArtifact {
                handle: ChildHandle {
                    relative_path: "bundle.js".into(),
                    hint: Some("javascript".into()),
                },
                bytes: src.as_bytes().to_vec(),
            });
        }

        Ok(children)
    }
}

/// Extract webpack modules using oxc parser.
/// Webpack 4: (function(modules){...})([fn1, fn2, ...])
/// Webpack 5: var __webpack_modules__ = {0: fn1, 1: fn2, ...}
fn extract_webpack_modules(src: &str) -> Vec<ChildArtifact> {
    let mut modules = Vec::new();

    let allocator = oxc_allocator::Allocator::default();
    let source_type = oxc_span::SourceType::unambiguous();
    let parser = oxc_parser::Parser::new(&allocator, src, source_type)
        .with_options(oxc_parser::ParseOptions {
            allow_return_outside_function: true,
            ..Default::default()
        });

    let ret = parser.parse();
    if ret.panicked || !ret.errors.is_empty() {
        return modules;
    }

    // Walk the AST looking for function modules.
    // Webpack 4: the IIFE is called with an array of functions.
    // Each function is a module: function(module, exports, require) { ... }
    // Webpack 5: __webpack_modules__ is an object with function values.

    // Simplified approach: use regex to find function(module, exports, __webpack_require__)
    // patterns and extract their bodies.
    let module_pattern = regex::Regex::new(
        r"(?ms)function\s*\(\s*\w+\s*,\s*\w+\s*,\s*\w+\s*\)\s*\{(.*?)(?:\}\s*[,)\]])"
    );

    if let Ok(re) = module_pattern {
        for (i, caps) in re.captures_iter(src).enumerate() {
            if let Some(body) = caps.get(1) {
                let module_code = format!("// webpack module {}\n(function(module, exports, require) {{\n{}\n}});",
                    i, body.as_str());
                modules.push(ChildArtifact {
                    handle: ChildHandle {
                        relative_path: format!("modules/{:03}.js", i),
                        hint: Some("javascript".into()),
                    },
                    bytes: module_code.into_bytes(),
                });
            }
        }
    }

    // Also try to extract the webpack runtime (the __webpack_require__ function)
    if let Some(idx) = src.find("__webpack_require__") {
        // Find the function containing __webpack_require__
        if let Some(fn_start) = src[..idx].rfind("function") {
            // Extract a chunk of the runtime
            let end = (idx + 500).min(src.len());
            let runtime = &src[fn_start..end];
            modules.push(ChildArtifact {
                handle: ChildHandle {
                    relative_path: "webpack-runtime.js".into(),
                    hint: Some("javascript".into()),
                },
                bytes: runtime.as_bytes().to_vec(),
            });
        }
    }

    modules
}
