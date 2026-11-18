package tray

import "testing"

func TestKeyAt(t *testing.T) {
	b := bar{
		items: []BarItem{{Key: "headset"}, {Key: "mouse"}},
		spans: []span{{4, 50}, {64, 110}},
	}
	cases := map[int]string{0: "headset", 20: "headset", 55: "headset", 60: "mouse", 90: "mouse", 200: "mouse"}
	for x, want := range cases {
		if got := b.keyAt(x, 0); got != want {
			t.Errorf("keyAt(%d) = %q, want %q", x, got, want)
		}
	}
	if got := (&bar{}).keyAt(10, 0); got != "" {
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

func TestRenderColumn(t *testing.T) {
	items := []BarItem{
		{Key: "a", Glyph: GlyphHeadset, Text: "40%", Fill: -1},
		{Key: "b", Glyph: GlyphMouse, Text: "100%", Charging: true, Fill: -1},
	}
	// Wide enough: one row per item. Narrow: glyph above level, so each item is taller.
	_, hWide, wide := renderColumn(items, 120, 96, false)
	_, hNarrow, narrow := renderColumn(items, 40, 96, false)
	if len(wide) != 2 || wide[0].x0 >= wide[0].x1 || wide[0].x1 > wide[1].x0 || wide[1].x1 > hWide {
		t.Fatalf("wide spans %+v (height %d)", wide, hWide)
	}
	if hNarrow <= hWide || narrow[1].x1 > hNarrow {
		t.Fatalf("narrow column not taller: wide %d narrow %d %+v", hWide, hNarrow, narrow)
	}
	b := bar{items: items, spans: wide, vertical: true}
	if got := b.keyAt(0, wide[1].x0+1); got != "b" {
		t.Errorf("vertical keyAt = %q", got)
	}
}
