package mdf

import "testing"

func TestThemeByNameExpanded(t *testing.T) {
	expected := []string{
		"kanagawa",
		"rose-pine",
		"rose-pine-dawn",
		"everforest",
		"everforest-light",
		"night-owl",
		"ayu-mirage",
		"ayu-light",
		"one-light",
		"one-dark",
		"solarized-light",
		"solarized-dark",
		"github-light",
		"github-dark",
		"papercolor-light",
		"papercolor-dark",
		"oceanic-next",
		"horizon",
		"palenight",
	}
	for _, name := range expected {
		if _, ok := ThemeByName(name); !ok {
			t.Fatalf("expected theme %q to be available", name)
		}
	}

	available := AvailableThemes()
	present := make(map[string]struct{}, len(available))
	for _, name := range available {
		present[name] = struct{}{}
	}
	for _, name := range expected {
		if _, ok := present[name]; !ok {
			t.Fatalf("expected theme %q in available list", name)
		}
	}
}
