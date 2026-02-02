package mdf

var asciiRuneStrings = func() [128]string {
	var out [128]string
	for i := 0; i < len(out); i++ {
		out[i] = string(rune(i))
	}
	return out
}()
