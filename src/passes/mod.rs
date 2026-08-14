// passes — pass registration center.
// Each language registers its Detector+Pass here.
// New languages only need to add a module and register in build_registry().

pub mod js;

use crate::core::registry::PassRegistry;

pub fn build_registry() -> PassRegistry {
    let mut r = PassRegistry::new();
    r.register(&js::JS_PASS);
    r
}
