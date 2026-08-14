// format.go provides source-level formatting for files that can't be fully
// parsed by goja (e.g. Monaco language definitions with heavy regex usage).
// It splits minified code into readable lines while preserving regex literals,
// strings, and comments verbatim.

package deobfuscator

// formatSourcePassthrough formats minified JS source by splitting on semicolons
// only, without AST analysis. Regex literals, strings, and comments are
// detected and preserved verbatim. Using only ; as a split point avoids
// breaking regex literals that contain { or }.
func formatSourcePassthrough(src string) string {
	var out []byte
	i := 0
	for i < len(src) {
		c := src[i]
		// Skip string literals.
		if c == '"' || c == '\'' {
			end := skipStringRaw(src, i)
			out = append(out, src[i:end]...)
			i = end
			continue
		}
		// Skip template literals.
		if c == '`' {
			end := skipTemplateRaw(src, i)
			out = append(out, src[i:end]...)
			i = end
			continue
		}
		// Skip comments.
		if c == '/' && i+1 < len(src) && src[i+1] == '/' {
			end := i
			for end < len(src) && src[end] != '\n' {
				end++
			}
			out = append(out, src[i:end]...)
			i = end
			continue
		}
		if c == '/' && i+1 < len(src) && src[i+1] == '*' {
			end := i + 2
			for end < len(src)-1 && !(src[end] == '*' && src[end+1] == '/') {
				end++
			}
			if end < len(src)-1 {
				end += 2
			} else {
				end = len(src)
			}
			out = append(out, src[i:end]...)
			i = end
			continue
		}
		// Detect regex literals: / after an operator/keyword/comma.
		if c == '/' && isRegexContextRaw(out) {
			end := skipRegexRaw(src, i)
			out = append(out, src[i:end]...)
			i = end
			continue
		}
		// Split on semicolons only (avoids breaking regex { } brackets).
		if c == ';' {
			out = append(out, c, '\n')
			i++
			continue
		}
		out = append(out, c)
		i++
	}
	return string(out)
}

func isRegexContextRaw(out []byte) bool {
	if len(out) == 0 {
		return true
	}
	// Regex follows operators, keywords, commas, parentheses, braces.
	c := out[len(out)-1]
	return c == '(' || c == ',' || c == '=' || c == ':' || c == '[' || c == '!' ||
		c == '&' || c == '|' || c == '<' || c == '>' || c == '+' || c == '-' ||
		c == '*' || c == '/' || c == '%' || c == '~' || c == '^' || c == '?' ||
		c == '{' || c == ';' || c == '\n' || c == ' ' || c == '\t'
}

func skipStringRaw(s string, i int) int {
	quote := s[i]
	i++
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			i += 2
			continue
		}
		if s[i] == quote {
			return i + 1
		}
		i++
	}
	return len(s)
}

func skipTemplateRaw(s string, i int) int {
	i++
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			i += 2
			continue
		}
		if s[i] == '`' {
			return i + 1
		}
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
			i += 2
			depth := 1
			for i < len(s) && depth > 0 {
				if s[i] == '{' {
					depth++
				} else if s[i] == '}' {
					depth--
				}
				i++
			}
			continue
		}
		i++
	}
	return len(s)
}

func skipRegexRaw(s string, i int) int {
	i++ // skip opening /
	inClass := false
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			i += 2
			continue
		}
		if s[i] == '[' {
			inClass = true
		}
		if s[i] == ']' {
			inClass = false
		}
		if s[i] == '/' && !inClass {
			i++
			for i < len(s) && ((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') || s[i] == '_' || s[i] == '$') {
				i++
			}
			return i
		}
		i++
	}
	return len(s)
}
