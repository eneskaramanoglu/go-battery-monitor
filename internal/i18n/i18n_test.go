package i18n

import "testing"

func TestTranslate(t *testing.T) {
	defer Set("en")
	Set("tr")
	if got := T("Exit"); got != "Çıkış" {
		t.Errorf("T(Exit) = %q", got)
	}
	if got := T("not in catalog"); got != "not in catalog" {
		t.Errorf("missing key = %q", got)
	}
	if got := F("Low battery: %s", "Mouse"); got != "Düşük pil: Mouse" {
		t.Errorf("F = %q", got)
	}
	Set("en")
	if got := T("Exit"); got != "Exit" {
		t.Errorf("english = %q", got)
	}
	Set("") // follows Windows: whatever it is, it must be a known language
	if c := Current(); c != "en" && c != "tr" {
		t.Errorf("system language = %q", c)
	}
}

// Every Turkish entry must have a non-empty translation.
func TestCatalogComplete(t *testing.T) {
	for k, v := range tr {
		if v == "" {
			t.Errorf("empty translation for %q", k)
		}
	}
}
