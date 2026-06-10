package mdf

// RenderOption configures rendering behavior.
type RenderOption func(*renderConfig)

type renderConfig struct {
	osc8            bool
	softWrap        bool
	tableBufferMode TableBufferMode
	tableWireMode   TableWireMode
	writeTrace      WriteTraceEncoder
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

// WithTableBufferMode configures how table renderers buffer rows.
// Invalid modes fall back to TableBufferFull.
func WithTableBufferMode(mode TableBufferMode) RenderOption {
	return func(cfg *renderConfig) {
		cfg.tableBufferMode = mode
	}
}

// WithTableWireMode configures the visible separators used for tables.
// Invalid modes fall back to TableWireLine.
func WithTableWireMode(mode TableWireMode) RenderOption {
	return func(cfg *renderConfig) {
		cfg.tableWireMode = mode
	}
}

// WithWriteTrace configures renderer emission tracing.
func WithWriteTrace(trace WriteTraceEncoder) RenderOption {
	return func(cfg *renderConfig) {
		cfg.writeTrace = trace
	}
}

func normalizeRenderConfig(cfg *renderConfig) {
	switch cfg.tableBufferMode {
	case TableBufferFull, TableBufferRow:
	default:
		cfg.tableBufferMode = TableBufferFull
	}
	switch cfg.tableWireMode {
	case TableWireLine, TableWireASCII, TableWireSpace:
	default:
		cfg.tableWireMode = TableWireLine
	}
}
