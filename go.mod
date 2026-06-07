module pkt.systems/mdf

go 1.25.0

require (
	github.com/muesli/reflow v0.3.0
	github.com/spf13/pflag v1.0.10
	golang.org/x/term v0.43.0
	pkt.systems/mdf/pdf/testdata v0.0.3
	pkt.systems/version v0.4.0
)

require (
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/mattn/go-runewidth v0.0.24 // indirect
	golang.org/x/sys v0.45.0 // indirect
)

retract (
	v0.0.2
	v0.0.1
	v0.0.1
)
