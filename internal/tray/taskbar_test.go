package tray

import "testing"

func TestKeyAt(t *testing.T) {
	b := bar{
		items: []BarItem{{Key: "headset"}, {Key: "mouse"}},
		spans: []span{{4, 50}, {64, 110}},
	}
	cases := map[int]string{0: "headset", 20: "headset", 55: "headset", 60: "mouse", 90: "mouse", 200: "mouse"}
	for x, want := range cases {
		if got := b.keyAt(x); got != want {
			t.Errorf("keyAt(%d) = %q, want %q", x, got, want)
		}
	}
	if got := (&bar{}).keyAt(10); got != "" {
		t.Errorf("empty bar keyAt = %q", got)
	}
}

func TestRenderBarSpans(t *testing.T) {
	items := []BarItem{{Key: "a", Glyph: GlyphHeadset, Text: "40%"}, {Key: "b", Glyph: GlyphMouse, Text: "85%", Charging: true}}
	_, w, spans := renderBar(items, 48, 96, false)
	if len(spans) != 2 || spans[0].x0 >= spans[0].x1 || spans[0].x1 >= spans[1].x0 || spans[1].x1 > w {
		t.Fatalf("spans %+v width %d", spans, w)
	}
}
