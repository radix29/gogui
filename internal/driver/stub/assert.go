package stub

import "github.com/radix29/gogui/internal/driver"

// Compile-time checks that every stub satisfies its peer interface.
var (
	_ driver.Widget      = (*Base)(nil)
	_ driver.Container   = (*Container)(nil)
	_ driver.Label       = (*Label)(nil)
	_ driver.Button      = (*Button)(nil)
	_ driver.Checkbox    = (*Checkbox)(nil)
	_ driver.Radio       = (*Radio)(nil)
	_ driver.TextField   = (*TextField)(nil)
	_ driver.Slider      = (*Slider)(nil)
	_ driver.Progress    = (*Progress)(nil)
	_ driver.PopUpButton = (*PopUpButton)(nil)
	_ driver.Separator   = (*Separator)(nil)
	_ driver.Box         = (*Box)(nil)
	_ driver.ScrollView  = (*ScrollView)(nil)
	_ driver.TextView    = (*TextView)(nil)
	_ driver.Window      = (*Window)(nil)
)
