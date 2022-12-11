package flagx

import (
	"flag"
	"fmt"
	"reflect"
	"strconv"
)

type StringField struct {
	f         *Field
	MaxLength *int
}

func NewStringField(f *Field) Fielder {
	return &StringField{f: f}
}

func (b *StringField) Init() error {
	return nil
}

func (b *StringField) BindOpt(key, value string) error {
	switch key {
	case KEY_MAX:
		length, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		b.MaxLength = &length
	default:
		return ErrOptMissHandler.Format(key, b.f.Name)
	}
	return nil
}

func (b *StringField) AfterParse() error {
	s := b.f.Instance().(*string)
	if b.MaxLength != nil {
		bb := []rune(*s)
		if len(bb) > *b.MaxLength {
			*s = string(bb[:*b.MaxLength])
		}
	}
	return nil
}

func (b *StringField) BindFlag(fs *flag.FlagSet) {
	ins := b.f.Instance().(*string)
	fs.StringVar(ins, b.f.FlagName(), b.f.DefVal, b.f.Usage)
}

func (b *StringField) Default() interface{} {
	return b.f.DefVal
}

func (b *StringField) SetArg(v *reflect.Value, arg string) error {
	if !v.CanSet() {
		return fmt.Errorf("value %v is not settable", v)
	}

	if arg == "" {
		arg = b.Default().(string)
	}
	v.Set(reflect.ValueOf(arg))
	return nil
}
