package sandbox

import (
	"testing"
	"time"
)

func TestRunSimple(t *testing.T) {
	setup := `
		var arr = ["hello", "world"];
		function decode(i) { return arr[i]; }
	`
	calls := []string{"decode(0)", "decode(1)"}
	out, err := Run(setup, calls, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 results, got %d", len(out))
	}
	if s, ok := DecodeString(out[0]); !ok || s != "hello" {
		t.Errorf("out[0] = %v, want hello", out[0])
	}
	if s, ok := DecodeString(out[1]); !ok || s != "world" {
		t.Errorf("out[1] = %v, want world", out[1])
	}
}

func TestRunRC4(t *testing.T) {
	// Simulate an obfuscator.io RC4 decoder with a custom charset.
	setup := `
		var chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
		var arr = ["aGVsbG8="];
		function b64_decode(input) {
			var output = '', tempEncStr = '';
			for (var bc = 0, bs = 0, buffer, idx = 0; (buffer = input.charAt(idx++)); ) {
				buffer = chars.indexOf(buffer);
				~buffer && ((bs = bc % 4 ? bs * 64 + buffer : buffer), bc++ % 4)
					? (output += String.fromCharCode(255 & (bs >> ((-2 * bc) & 6))))
					: 0;
			}
			for (var k = 0, length = output.length; k < length; k++) {
				tempEncStr += '%' + ('00' + output.charCodeAt(k).toString(16)).slice(-2);
			}
			return decodeURIComponent(tempEncStr);
		}
		function decode(i) { return b64_decode(arr[i]); }
	`
	calls := []string{"decode(0)"}
	out, err := Run(setup, calls, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := DecodeString(out[0]); !ok || s != "hello" {
		t.Errorf("out[0] = %v, want hello", out[0])
	}
}

func TestRunTimeout(t *testing.T) {
	setup := `function f() { while(true){} return 1; }`
	calls := []string{"f()"}
	_, err := Run(setup, calls, 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
