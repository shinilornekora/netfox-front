package principal

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestSignatureBoundToRequest(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	s := Signer{Key: []byte("k"), Now: func() time.Time { return now }}
	p := Principal{UserID: "u1", Role: Viewer, BirthYear: 2000}

	orig := httptest.NewRequest("GET", "/api/playback/history", nil)
	s.Stamp(orig, p)
	if got, ok, err := s.Read(orig); err != nil || !ok || got != p {
		t.Fatalf("own request: %+v %v %v", got, ok, err)
	}

	cases := map[string]struct{ method, path string }{
		"другой метод": {"DELETE", "/api/playback/history"},
		"другой путь":  {"GET", "/api/billing/orders"},
	}
	for name, c := range cases {
		r := httptest.NewRequest(c.method, c.path, nil)
		r.Header = orig.Header.Clone()
		if _, _, err := s.Read(r); err == nil {
			t.Errorf("%s: подпись принята", name)
		}
	}

	// Подмена роли ломает подпись.
	forged := httptest.NewRequest("GET", "/api/playback/history", nil)
	forged.Header = orig.Header.Clone()
	forged.Header.Set(HRole, string(Admin))
	if _, _, err := s.Read(forged); err == nil {
		t.Error("подменённая роль принята")
	}

	// Через 61 секунду подпись устаревает.
	now = now.Add(MaxSkew + time.Second)
	if _, _, err := s.Read(orig); err == nil {
		t.Error("устаревшая подпись принята")
	}
}
