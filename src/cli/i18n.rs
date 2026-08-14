// i18n — auto-detect system locale, return English or Chinese strings.

use std::env;

/// Returns true if the system locale is Chinese.
pub fn is_chinese() -> bool {
    for var in &["LANG", "LC_ALL", "LC_MESSAGES"] {
        if let Ok(val) = env::var(var) {
            if val.contains("zh") || val.contains("ZH") || val.contains("Chinese") {
                return true;
            }
        }
    }
    false
}

/// Returns English or Chinese text based on system locale.
pub fn t(en: &str, zh: &str) -> &'static str {
    // Leak to get 'static lifetime for dynamic strings.
    // In practice the number of calls is small (CLI help text).
    if is_chinese() {
        Box::leak(zh.to_string().into_boxed_str())
    } else {
        Box::leak(en.to_string().into_boxed_str())
    }
}
