// passes — pass registration center.
// Each language registers its Detector+Pass here.
// New languages only need to: add a module, implement Pass+Detector, register.

pub mod js;
pub mod asar;
pub mod python;
pub mod native;
pub mod jvm;
pub mod wasm;

use crate::core::registry::PassRegistry;

pub fn build_registry() -> PassRegistry {
    let mut r = PassRegistry::new();
    r.register(&asar::ASAR_PASS);     // Electron asar unpacking (fully implemented)
    r.register(&js::JS_PASS);         // JavaScript deobfuscation (fully implemented)
    r.register(&python::PY_PASS);     // Python .pyc (detector only)
    r.register(&native::NATIVE_PASS); // PE/ELF/Mach-O (detector only)
    r.register(&jvm::JVM_PASS);       // .class/DEX (detector only)
    r.register(&wasm::WASM_PASS);     // WebAssembly (detector only)
    r
}
