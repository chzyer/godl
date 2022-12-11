package flagx

import "flag"

type BoolField struct {
	f      *Field
	defval bool
}

func NewBoolField(f *Field) Fielder {

	return &BoolField{
		f: f,
	}
}

func (b *BoolField) Init() error {
	b.defval = true
	if b.f.DefVal == "" {
		b.defval = false
	}
	return nil
}

func (b *BoolField) BindFlag(fs *flag.FlagSet) {
	fs.BoolVar(b.f.Instance().(*bool), b.f.FlagName(), b.defval, b.f.Usage)
}

func (b *BoolField) Default() interface{} {
	return b.defval
}
