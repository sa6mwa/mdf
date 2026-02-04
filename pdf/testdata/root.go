package testdata

import (
	"errors"
	"path/filepath"
	"runtime"
)

// Root returns the absolute path to the pdf testdata module directory.
func Root() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("testdata: unable to resolve module path")
	}
	return filepath.Dir(file), nil
}
