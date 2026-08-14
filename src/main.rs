// jd — multi-language deobfuscation framework.
// Architecture inspired by disrobe: Pass/Detector/Registry for plugin-style
// multi-language support. New languages only need to implement Pass + Detector
// and register in passes::build_registry().
mod cli;
mod core;
mod detect;
mod deobfuscate;
mod codegen;
mod passes;
mod sandbox;
mod unminify;

fn main() {
    cli::run();
}
