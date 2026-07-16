package assets

import (
	"bytes"
	"image/png"
	"testing"
)

// @description    Verifies that the warning icon is embedded as valid PNG data.
//
// @param           t  "test context"
func TestWarningPNGIsEmbedded(t *testing.T) {
	t.Parallel()

	if len(WarningPNG) == 0 {
		t.Fatal("warning icon is empty")
	}

	if _, err := png.Decode(bytes.NewReader(WarningPNG)); err != nil {
		t.Fatalf("warning icon is not valid PNG data: %v", err)
	}
}
