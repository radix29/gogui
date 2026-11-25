package menubar

// Titles returns the menus' titles in strip order.
func (b *Bar) Titles() []string {
	if b == nil {
		return nil
	}
	out := make([]string, len(b.Menus))
	for i, m := range b.Menus {
		out[i] = m.Title
	}
	return out
}
