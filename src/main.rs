// jd — JavaScript deobfuscator (obfuscator.io + unminify)
// Rust port of webcrack and synchrony, inspired by disrobe's architecture.
mod cli;
mod detect;
mod deobfuscate;
mod unminify;
mod sandbox;
mod codegen;

fn main() {
    cli::run();
}
