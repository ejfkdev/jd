// passes — pass registration center.
// Each language registers its Detector+Pass here.
// New languages only need to: add a module, implement Pass+Detector, register.
// Reference: disrobe-passes/src/lib.rs::build_registry().

pub mod js;
pub mod python;
pub mod native;
pub mod jvm;
pub mod wasm;

use crate::core::registry::PassRegistry;

/// Build and return the registry of all registered passes.
pub fn build_registry() -> PassRegistry {
    let mut r = PassRegistry::new();
    r.register(&js::JS_PASS);           // JavaScript deobfuscation (fully implemented)
    r.register(&python::PY_PASS);       // Python .pyc (detector only — TODO: decompile)
    r.register(&native::NATIVE_PASS);  // PE/ELF/Mach-O (detector only — TODO: unpack)
    r.register(&jvm::JVM_PASS);        // .class/DEX (detector only — TODO: decompile)
    r.register(&wasm::WASM_PASS);      // WebAssembly (detector only — TODO: decompile)
    r
}
