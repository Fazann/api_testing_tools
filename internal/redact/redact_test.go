package redact

import (
	"net/http"
	"strings"
	"testing"

	"api-test-kits/internal/domain"
)

func TestStringRedactsAllSecrets(t *testing.T) {
	r := New("s3cr3t-token", "hunter22", "")
	out := r.String(`token=s3cr3t-token and pw "hunter22" twice: hunter22`)
	if strings.Contains(out, "s3cr3t") || strings.Contains(out, "hunter22") {
		t.Fatalf("secret leaked: %s", out)
	}
	if strings.Count(out, Mask) != 3 {
		t.Fatalf("expected 3 masks, got %s", out)
	}
}

func TestOverlappingSecretsLongestFirst(t *testing.T) {
	r := New("abcd", "abcdefgh")
	if out := r.String("xx abcdefgh yy"); strings.Contains(out, "efgh") {
		t.Fatalf("longer secret partially leaked: %s", out)
	}
}

func TestURLEncodedSecret(t *testing.T) {
	r := New("p@ss word/1")
	out := r.URL("https://api.test/x?key=p%40ss+word%2F1&path=ok")
	if strings.Contains(out, "p%40ss") || strings.Contains(out, "p@ss") {
		t.Fatalf("encoded secret leaked: %s", out)
	}
}

func TestShortSecretsOnlyWholeValues(t *testing.T) {
	r := New("ab")
	if got := r.String("about tabs"); got != "about tabs" {
		t.Fatalf("short secret must not mangle text: %s", got)
	}
	if got := r.String("ab"); got != Mask {
		t.Fatalf("short secret as whole value must be masked: %s", got)
	}
	if got := r.URL("https://h/x?k=ab&q=about"); strings.Contains(got, "k=ab") || !strings.Contains(got, "q=about") {
		t.Fatalf("got %s", got)
	}
}

func TestSensitiveHeadersAlwaysMasked(t *testing.T) {
	r := New()
	r.AddSensitiveHeader("x-custom-key")
	h := http.Header{}
	h.Set("Authorization", "Bearer whatever")
	h.Set("Cookie", "sid=1")
	h.Set("X-Custom-Key", "k")
	h.Set("Accept", "application/json")
	for _, kv := range r.HTTPHeader(h) {
		if kv.Key == "Accept" {
			if kv.Value != "application/json" {
				t.Fatalf("non-sensitive header changed: %+v", kv)
			}
			continue
		}
		if kv.Value != Mask {
			t.Fatalf("header %s not masked: %s", kv.Key, kv.Value)
		}
	}
}

func TestUserInfoRemovedFromURL(t *testing.T) {
	if got := New().URL("https://user:pw@host/x"); strings.Contains(got, "pw") {
		t.Fatalf("userinfo leaked: %s", got)
	}
}

func TestHeadersCopy(t *testing.T) {
	in := []domain.KeyValue{{Key: "Authorization", Value: "x", Enabled: true}}
	out := New().Headers(in)
	if in[0].Value != "x" || out[0].Value != Mask {
		t.Fatal("Headers must return a redacted copy without modifying input")
	}
}
