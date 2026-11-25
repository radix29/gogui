package theme_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/theme"
)

// TestAlphaTokensFloat32 (review R11): every alpha and shade token is a
// float32, the type of the color channel it scales, in every group.
func TestAlphaTokensFloat32(t *testing.T) {
	n := 0
	th := reflect.TypeFor[theme.Theme]()
	for i := range th.NumField() {
		g := th.Field(i)
		if g.Type.Kind() != reflect.Struct {
			continue
		}
		for j := range g.Type.NumField() {
			f := g.Type.Field(j)
			if !strings.HasSuffix(f.Name, "Alpha") && !strings.HasSuffix(f.Name, "Shade") {
				continue
			}
			n++
			if f.Type.Kind() != reflect.Float32 {
				t.Errorf("%s.%s is %v, want float32", g.Name, f.Name, f.Type)
			}
		}
	}
	if n < 9 {
		t.Errorf("found %d alpha and shade tokens, want at least 9", n)
	}
}
