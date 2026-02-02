package mdf

import (
	"os"
	"strconv"
	"strings"
)

const (
	osc8Start = "\x1b]8;;"
	osc8End   = "\x1b]8;;\x1b\\"
)

// DetectOSC8Support returns true if the current environment likely supports OSC 8 hyperlinks.
func DetectOSC8Support() bool {
	if os.Getenv("OSC8") == "0" {
		return false
	}
	if os.Getenv("DOMTERM") != "" {
		return true
	}
	if os.Getenv("WT_SESSION") != "" {
		return true
	}
	termProgram := os.Getenv("TERM_PROGRAM")
	if termProgram == "iTerm.app" || termProgram == "WezTerm" || termProgram == "vscode" {
		return true
	}
	if strings.Contains(strings.ToLower(os.Getenv("TERM")), "kitty") {
		return true
	}
	if vte := os.Getenv("VTE_VERSION"); vte != "" {
		if n, err := strconv.Atoi(vte); err == nil && n >= 5000 {
			return true
		}
	}
	return false
}
