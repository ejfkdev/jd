package decoder

import "testing"

// 64-char charset without padding (matches obfuscator.io's decoder semantics
// where padding is handled by bc-alignment, not a padding symbol).
var stdCharset64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

func TestBase64DecodeSimple(t *testing.T) {
	std := stdCharset64
	cases := map[string]string{
		"aGVsbG8":             "hello",
		"d29ybGQ":             "world",
		"Zm9v":                "foo",
		"YmFy":                "bar",
		"YmF6":                "baz",
		"VGhpcyBpcyBhIHRlc3Q": "This is a test",
	}
	for in, want := range cases {
		if got := Base64Decode(std, in); got != want {
			t.Errorf("Base64Decode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRC4RoundTrip(t *testing.T) {
	plaintext := "secret message"
	key := "mykey"
	enc := rc4Encode(stdCharset64, plaintext, key)
	dec := RC4Decode(stdCharset64, enc, key)
	if dec != plaintext {
		t.Errorf("RC4 round-trip failed: got %q, want %q", dec, plaintext)
	}
}

func rc4Encode(charset, input, key string) string {
	enc := rc4Core(input, key)
	return base64Encode(charset, enc)
}

func rc4Core(input, key string) string {
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
	i, jj := 0, 0
	for y := 0; y < len(input); y++ {
		i = (i + 1) % 256
		jj = (jj + s[i]) % 256
		s[i], s[jj] = s[jj], s[i]
		out = append(out, input[y]^byte(s[(s[i]+s[jj])%256]))
	}
	return string(out)
}

func base64Encode(charset, input string) string {
	var out []byte
	for i := 0; i < len(input); i += 3 {
		b := []byte{0, 0, 0}
		n := copy(b, input[i:])
		val := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
		out = append(out, charset[(val>>18)&0x3F])
		out = append(out, charset[(val>>12)&0x3F])
		if n > 1 {
			out = append(out, charset[(val>>6)&0x3F])
		}
		if n > 2 {
			out = append(out, charset[val&0x3F])
		}
	}
	return string(out)
}

func TestSimpleDecode(t *testing.T) {
	arr := []string{"zero", "one", "two"}
	if v, ok := SimpleDecode(arr, 1); !ok || v != "one" {
		t.Errorf("SimpleDecode(1) = %q %v, want one true", v, ok)
	}
	if _, ok := SimpleDecode(arr, 5); ok {
		t.Error("SimpleDecode(5) should be out of range")
	}
}

func TestRotate(t *testing.T) {
	// Simulate a rotation where breakCond=3 and the chain evaluates arr[0]
	// (the first element). Starting array [0,1,2,3,4], after 3 push/shifts
	// the first element becomes 3.
	arr := []string{"0", "1", "2", "3", "4"}
	out, ok := Rotate(arr, 3, 10, func(a []string) (float64, bool) {
		v, b := parseFloat(a[0])
		return v, b
	})
	if !ok {
		t.Fatal("rotation did not converge")
	}
	if out[0] != "3" {
		t.Errorf("after rotation, first element = %q, want 3", out[0])
	}
}

func parseFloat(s string) (float64, bool) {
	var f float64
	for _, c := range s {
		f = f*10 + float64(c-'0')
	}
	return f, true
}
