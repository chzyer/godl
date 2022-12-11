package flagx

import "reflect"

var (
	IntFieldHook    = Hooks{}
	BoolFieldHook   = Hooks{}
	StringFieldHook = Hooks{}
	SliceFieldHook  = Hooks{}
)

type Hooks []*Hook

func (hs Hooks) Select(t reflect.Type) func(t *Field) Fielder {
	for _, h := range hs {
		if h.Fit(t) {
			return h.New
		}
	}
	return nil
}

func (hs *Hooks) Append(h *Hook) {
	*hs = append(*hs, h)
}

type Hook struct {
	Fit func(t reflect.Type) bool
	New func(t *Field) Fielder
}
