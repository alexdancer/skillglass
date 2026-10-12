package terminal

import "testing"

func TestResolveBrowserThemeAndBackgroundHint(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("COLORFGBG", "15;0")
	theme, err := resolveBrowserTheme("auto")
	if err != nil || theme != themeDark {
		t.Fatalf("dark auto theme = %v, %v", theme, err)
	}
	t.Setenv("COLORFGBG", "0;15")
	theme, err = resolveBrowserTheme("auto")
	if err != nil || theme != themeLight {
		t.Fatalf("light auto theme = %v, %v", theme, err)
	}
	t.Setenv("NO_COLOR", "1")
	theme, err = resolveBrowserTheme("dark")
	if err != nil || theme != themeNone {
		t.Fatalf("NO_COLOR theme = %v, %v", theme, err)
	}
}
