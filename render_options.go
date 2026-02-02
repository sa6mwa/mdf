package mdf

// RenderOption configures rendering behavior.
type RenderOption func(*renderConfig)

type renderConfig struct {
	osc8     bool
	softWrap bool
}

// WithOSC8 enables or disables OSC 8 hyperlinks.
func WithOSC8(enabled bool) RenderOption {
	return func(cfg *renderConfig) {
		cfg.osc8 = enabled
	}
}

// WithSoftWrap enables soft wrapping for long words.
func WithSoftWrap(enabled bool) RenderOption {
	return func(cfg *renderConfig) {
		cfg.softWrap = enabled
	}
}
