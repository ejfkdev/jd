// core/chain — ChainDriver state machine for automatic onion-peeling.
// When input is asar → extract files → JS files auto-deobfuscate → bundles auto-unpack.
// Reference: disrobe-core/src/chain/state_machine.rs (simplified).

use std::collections::{BTreeSet, VecDeque};
use crate::core::artifact::{Artifact, OutputKind, ChildArtifact, TERMINAL_HINT, Language};
use crate::core::detect::{DetectContext, DetectVerdict};
use crate::core::registry::{PassRegistry, DetectorPick};

const DEFAULT_CAP: u8 = 8;
const MAX_CAP: u8 = 16;

/// Chain specification — how deep to go.
#[derive(Debug, Clone)]
pub enum ChainSpec {
    Auto { cap: u8 },
    Explicit { pass_id: String },
}

impl ChainSpec {
    pub fn auto() -> Self { Self::Auto { cap: DEFAULT_CAP } }
    pub fn auto_with_cap(cap: u8) -> Self { Self::Auto { cap: cap.min(MAX_CAP) } }
    pub fn cap(&self) -> u8 {
        match self { Self::Auto { cap } => *cap, Self::Explicit { .. } => 1 }
    }
}

/// A recovered child file from the chain.
#[derive(Debug)]
pub struct RecoveredFile {
    pub relative_path: String,
    pub content: Vec<u8>,
    pub depth: u8,
    pub pass_id: &'static str,
}

/// The chain plan — output of running the chain.
#[derive(Debug)]
pub struct ChainPlan {
    pub recovered_files: Vec<RecoveredFile>,
    pub final_output: Vec<u8>,
    pub layers: Vec<ChainLayer>,
}

#[derive(Debug)]
pub struct ChainLayer {
    pub depth: u8,
    pub pass_id: &'static str,
    pub input_size: usize,
    pub output_size: usize,
    pub children_count: usize,
}

/// Work item in the BFS queue.
struct WorkItem {
    bytes: Vec<u8>,
    relative_path: String,
    depth: u8,
    history: BTreeSet<u64>,
    hint: Option<String>,
}

/// The chain driver — runs the onion-peeling BFS loop.
pub struct ChainDriver<'r> {
    pub registry: &'r PassRegistry,
}

impl<'r> ChainDriver<'r> {
    pub fn new(registry: &'r PassRegistry) -> Self {
        Self { registry }
    }

    /// Run the chain on input bytes. Returns recovered files and final output.
    pub fn run(&self, input: &[u8], spec: &ChainSpec) -> ChainPlan {
        let cap = spec.cap();
        let mut queue: VecDeque<WorkItem> = VecDeque::new();
        let mut seen: BTreeSet<u64> = BTreeSet::new();
        let mut recovered_files = Vec::new();
        let mut layers = Vec::new();
        let mut final_output = Vec::new();

        // Seed with input
        let input_hash = simple_hash(input);
        seen.insert(input_hash);
        queue.push_back(WorkItem {
            bytes: input.to_vec(),
            relative_path: String::new(),
            depth: 0,
            history: { let mut h = BTreeSet::new(); h.insert(input_hash); h },
            hint: None,
        });

        while let Some(item) = queue.pop_front() {
            // Depth cap
            if item.depth >= cap {
                // Save as recovered file (terminal at depth limit)
                if !item.relative_path.is_empty() {
                    recovered_files.push(RecoveredFile {
                        relative_path: item.relative_path,
                        content: item.bytes.clone(),
                        depth: item.depth,
                        pass_id: "depth-cap",
                    });
                } else {
                    final_output = item.bytes.clone();
                }
                continue;
            }

            // Detect format
            let ctx = DetectContext {
                bytes: &item.bytes,
                path_hint: Some(&item.relative_path),
                depth: item.depth,
            };
            let pick = match self.registry.run_all_and_pick(&ctx) {
                Some(p) => p,
                None => {
                    // No detector matched — this is terminal (source code, non-code file)
                    if !item.relative_path.is_empty() {
                        recovered_files.push(RecoveredFile {
                            relative_path: item.relative_path,
                            content: item.bytes.clone(),
                            depth: item.depth,
                            pass_id: "terminal",
                        });
                    } else {
                        final_output = item.bytes.clone();
                    }
                    continue;
                }
            };

            // Run the pass
            let artifact = Artifact::new_raw(item.bytes.clone());
            let result = match pick.pass.run(&artifact) {
                Ok(a) => a,
                Err(_) => {
                    // Pass failed — save input as-is
                    if !item.relative_path.is_empty() {
                        recovered_files.push(RecoveredFile {
                            relative_path: item.relative_path,
                            content: item.bytes.clone(),
                            depth: item.depth,
                            pass_id: pick.verdict.pass_id,
                        });
                    } else {
                        final_output = item.bytes.clone();
                    }
                    continue;
                }
            };

            // Determine output kind
            let output_kind = pick.pass.output_kind(&result);
            let output_bytes = &result.bytes;

            // Cycle detection
            let output_hash = simple_hash(output_bytes);
            if item.history.contains(&output_hash) || seen.contains(&output_hash) {
                // Cycle — save as terminal
                if !item.relative_path.is_empty() {
                    recovered_files.push(RecoveredFile {
                        relative_path: item.relative_path,
                        content: output_bytes.clone(),
                        depth: item.depth,
                        pass_id: pick.verdict.pass_id,
                    });
                } else {
                    final_output = output_bytes.clone();
                }
                continue;
            }
            seen.insert(output_hash);

            layers.push(ChainLayer {
                depth: item.depth,
                pass_id: pick.verdict.pass_id,
                input_size: item.bytes.len(),
                output_size: output_bytes.len(),
                children_count: 0,
            });

            // Dispatch on output kind
            match output_kind {
                OutputKind::Source { .. } => {
                    // Terminal — recovered source
                    if !item.relative_path.is_empty() {
                        recovered_files.push(RecoveredFile {
                            relative_path: item.relative_path,
                            content: output_bytes.clone(),
                            depth: item.depth,
                            pass_id: pick.verdict.pass_id,
                        });
                    } else {
                        final_output = output_bytes.clone();
                    }
                }
                OutputKind::Bytes { .. } => {
                    // Re-queue for next layer
                    let mut new_history = item.history.clone();
                    new_history.insert(output_hash);
                    queue.push_back(WorkItem {
                        bytes: output_bytes.clone(),
                        relative_path: item.relative_path.clone(),
                        depth: item.depth + 1,
                        history: new_history,
                        hint: None,
                    });
                }
                OutputKind::Mixed { .. } => {
                    // Fan-out: extract children
                    let children = pick.pass.extract_children(&artifact).unwrap_or_default();
                    for child in children {
                        if child.handle.hint.as_deref() == Some(TERMINAL_HINT) {
                            // Terminal child — save directly
                            recovered_files.push(RecoveredFile {
                                relative_path: child.handle.relative_path,
                                content: child.bytes,
                                depth: item.depth + 1,
                                pass_id: pick.verdict.pass_id,
                            });
                        } else {
                            // Re-queue child for further processing
                            let child_hash = simple_hash(&child.bytes);
                            if !seen.contains(&child_hash) {
                                seen.insert(child_hash);
                                let mut new_history = item.history.clone();
                                new_history.insert(child_hash);
                                queue.push_back(WorkItem {
                                    bytes: child.bytes,
                                    relative_path: child.handle.relative_path,
                                    depth: item.depth + 1,
                                    history: new_history,
                                    hint: child.handle.hint,
                                });
                            }
                        }
                    }
                }
            }
        }

        // If no output was set, use input
        if final_output.is_empty() && !input.is_empty() && recovered_files.is_empty() {
            final_output = input.to_vec();
        }

        ChainPlan { recovered_files, final_output, layers }
    }
}

/// Simple hash for cycle detection (not cryptographic — just dedup).
fn simple_hash(bytes: &[u8]) -> u64 {
    use std::collections::hash_map::DefaultHasher;
    use std::hash::{Hash, Hasher};
    let mut hasher = DefaultHasher::new();
    bytes.hash(&mut hasher);
    hasher.finish()
}
