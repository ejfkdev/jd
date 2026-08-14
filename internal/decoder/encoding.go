// Package decoder implements the static decoding path for obfuscator.io
// string arrays: array-rotation simulation (push/shift) and the three built-in
// encodings (SIMPLE, Base64 with custom charset, RC4). These algorithms are a
// direct port of synchrony's util_b64_decode / util_rc4_decode.
package decoder

// Encoding classifies a decoder function's algorithm.
type Encoding int

const (
	EncSimple  Encoding = iota // plain array lookup
	EncBase64                  // custom-charset base64
	EncRC4                     // RC4 after base64 pre-decode
	EncUnknown                 // not statically decodable; needs sandbox
)

// Base64Decode reimplements javascript-obfuscator's custom-charset base64.
// The charset is 65 characters: 64 alphabet symbols plus a padding char
// (typically '=') at index 64. The algorithm exactly mirrors the JS for-loop:
//
//	for (bc=0,bs=0,buffer,idx=0; buffer=input.charAt(idx++); ) {
//	  buffer = charset.indexOf(buffer)
//	  if (~buffer) {
//	    bs = bc%4 ? bs*64+buffer : buffer
//	    bc++  // post-increment: the % test below uses the OLD bc
//	    if (old_bc % 4) output += fromCharCode(255 & (bs >> ((-2*new_bc)&6)))
//	  }
//	}
//	then: percent-encode each output byte as %XX and decodeURIComponent
func Base64Decode(charset, input string) string {
	bc := 0
	bs := 0
	var out []byte
	for i := 0; i < len(input); i++ {
		buffer := indexOf(charset, input[i])
		if buffer < 0 { // ~buffer == 0, skip
			continue
		}
		oldBc := bc
		if bc%4 != 0 {
			bs = bs*64 + buffer
		} else {
			bs = buffer
		}
		bc++ // post-increment
		if oldBc%4 != 0 {
			out = append(out, byte(255&(bs>>((-2*bc)&6))))
		}
	}
	return string(out)
}

// indexOf returns the index of b in s, or -1.
func indexOf(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// RC4Decode reimplements javascript-obfuscator's RC4. It first Base64-decodes
// the input using charset, then runs standard RC4 with the UTF-16 code units of
// key. Both the key and the decoded bytes operate on 8-bit values (decoded
// bytes are ≤255 so their UTF-16 code units equal their byte values).
func RC4Decode(charset, input, key string) string {
	decoded := Base64Decode(charset, input)
	// Key to UTF-16 code units.
	keyUnits := utf16Encode(key)

	s := make([]int, 256)
	for i := 0; i < 256; i++ {
		s[i] = i
	}
	j := 0
	for i := 0; i < 256; i++ {
		j = (j + s[i] + keyUnits[i%len(keyUnits)]) % 256
		s[i], s[j] = s[j], s[i]
	}

	var out []byte
	i, j := 0, 0
	for y := 0; y < len(decoded); y++ {
		i = (i + 1) % 256
		j = (j + s[i]) % 256
		s[i], s[j] = s[j], s[i]
		out = append(out, decoded[y]^byte(s[(s[i]+s[j])%256]))
	}
	return string(out)
}

// utf16Encode converts a Go string (UTF-8) to UTF-16 code units.
func utf16Encode(s string) []int {
	var out []int
	for _, r := range s {
		if r > 0xFFFF {
			// surrogate pair
			r -= 0x10000
			out = append(out, 0xD800+(int(r>>10)))
			out = append(out, 0xDC00+(int(r&0x3FF)))
		} else {
			out = append(out, int(r))
		}
	}
	return out
}

// SimpleDecode returns strings[index]. Returns "" and false when index is out
// of range.
func SimpleDecode(strings []string, index int) (string, bool) {
	if index < 0 || index >= len(strings) {
		return "", false
	}
	return strings[index], true
}

// Rotate simulates the obfuscator.io array-rotation IIFE: repeatedly
// push(shift()) until the parseInt-chain equals breakCond. Returns the rotated
// array and whether the break condition was reached within maxLoops.
//
// decodeFn decodes an individual parseInt(decoder(args)) call using the
// current array state; it returns (value, ok) where ok=false means "unknown /
// NaN" (skip this iteration).
func Rotate(strings []string, breakCond float64, maxLoops int, evalChain func(arr []string) (float64, bool)) ([]string, bool) {
	arr := make([]string, len(strings))
	copy(arr, strings)
	for i := 0; i < maxLoops; i++ {
		v, ok := evalChain(arr)
		if ok && v == breakCond {
			return arr, true
		}
		// push(shift())
		first := arr[0]
		copy(arr, arr[1:])
		arr[len(arr)-1] = first
	}
	return arr, false
}
