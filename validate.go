package mdf

import (
	"errors"
	"unicode/utf8"
)

var (
	// ErrInvalidUTF8 reports invalid UTF-8 input.
	ErrInvalidUTF8 = errors.New("invalid utf-8 input")
	// ErrBinaryInput reports input that appears to be binary.
	ErrBinaryInput = errors.New("binary input detected")
)

const (
	minBinarySample = 64
	maxControlPct   = 2
)

// ValidateInput returns an error if the input is not valid UTF-8 or appears binary.
func ValidateInput(src []byte) error {
	if !utf8.Valid(src) {
		return ErrInvalidUTF8
	}
	var total, control int
	for _, b := range src {
		total++
		if b == 0x00 {
			return ErrBinaryInput
		}
		if isControlByte(b) {
			control++
		}
	}
	if total >= minBinarySample && control*100 >= total*maxControlPct {
		return ErrBinaryInput
	}
	return nil
}

type validator struct {
	total   int
	control int
}

func (v *validator) reset() {
	v.total = 0
	v.control = 0
}

func (v *validator) addBytes(b []byte) ([]byte, error) {
	i := 0
	for i < len(b) {
		if !utf8.FullRune(b[i:]) {
			break
		}
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size == 1 {
			return nil, ErrInvalidUTF8
		}
		if size == 0 || i+size > len(b) {
			break
		}
		if err := v.addRune(r, size); err != nil {
			return nil, err
		}
		i += size
	}
	return b[i:], nil
}

func (v *validator) addRune(r rune, size int) error {
	if r == utf8.RuneError && size == 1 {
		return ErrInvalidUTF8
	}
	if r == 0 {
		return ErrBinaryInput
	}
	v.total += size
	if isControlRune(r) {
		v.control++
		if v.total >= minBinarySample && v.control*100 >= v.total*maxControlPct {
			return ErrBinaryInput
		}
	}
	return nil
}

func isControlByte(b byte) bool {
	if b < 0x09 {
		return true
	}
	if b > 0x0D && b < 0x20 {
		return true
	}
	if b == 0x7F {
		return true
	}
	return false
}

func isControlRune(r rune) bool {
	if r == '\n' || r == '\r' || r == '\t' {
		return false
	}
	if r < 0x20 || r == 0x7F {
		return true
	}
	return false
}

func sanitizeBytes(dst []byte, src []byte) ([]byte, []byte) {
	di := 0
	i := 0
	for i < len(src) {
		if !utf8.FullRune(src[i:]) {
			break
		}
		r, size := utf8.DecodeRune(src[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		if size == 0 || i+size > len(src) {
			break
		}
		if isControlRune(r) {
			i += size
			continue
		}
		copy(dst[di:], src[i:i+size])
		di += size
		i += size
	}
	return dst[:di], src[i:]
}
