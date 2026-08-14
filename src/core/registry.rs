// PassRegistry — register passes, run detectors, pick winners.
use std::collections::BTreeMap;
use super::pass::{Pass, PassId};
use super::detect::{DetectContext, DetectVerdict};

pub struct DetectorPick { pub pass: &'static dyn Pass, pub verdict: DetectVerdict }

pub struct PassRegistry { passes: BTreeMap<PassId, &'static dyn Pass> }

impl PassRegistry {
    pub fn new() -> Self { Self { passes: BTreeMap::new() } }
    pub fn register(&mut self, pass: &'static dyn Pass) { self.passes.insert(pass.id(), pass); }
    pub fn len(&self) -> usize { self.passes.len() }

    pub fn run_all_and_pick(&self, ctx: &DetectContext) -> Option<DetectorPick> {
        let mut best: Option<(f32, PassId)> = None;
        for &pass in self.passes.values() {
            if let Some(v) = pass.detector().detect(ctx) {
                if best.as_ref().map_or(true, |(c, _)| v.confidence > *c) {
                    best = Some((v.confidence, pass.id()));
                }
            }
        }
        best.and_then(|(_, id)| {
            let pass = *self.passes.get(&id)?;
            pass.detector().detect(ctx).map(|v| DetectorPick { pass, verdict: v })
        })
    }
}

impl Default for PassRegistry { fn default() -> Self { Self::new() } }
