package flagx

import (
	"flag"
	"fmt"
	"reflect"
	"strconv"
)

type SliceSetter struct {
	Val  *reflect.Value
	Type reflect.Type
}

func NewSliceSetter(val *reflect.Value, t reflect.Type) *SliceSetter {
	return &SliceSetter{
		Val:  val,
		Type: t,
	}
}

func (ss *SliceSetter) Set(s string) error {
	ss.Val.Set(reflect.Append(*ss.Val, reflect.ValueOf(s)))
	return nil
}

func (s *SliceSetter) String() string {
	return fmt.Sprintf("%v", s.Val.Interface())
}

type SliceField struct {
	f   *Field
	Min *int
}

func NewSliceField(f *Field) Fielder {
	return &SliceField{f: f}
}

func (b *SliceField) Init() error {
	switch b.f.Type.Elem().Kind() {
	case reflect.String:
	default:
		return ErrTypeSupport.Format(b.f.Type)
	}
	return nil
}

func (b *SliceField) SetArgs(v *reflect.Value, args []string) error {
	if b.Min != nil {
		if len(args) < *b.Min {
			return ErrUsage.Formatf("length of args must more than %v", *b.Min)
		}
	}
	*(b.f.Instance().(*[]string)) = args
	return nil
}

func (b *SliceField) BindOpt(k, v string) (err error) {
	switch k {
	case KEY_MIN:
		min, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		b.Min = &min
	default:
		return ErrOptMissHandler.Format(k, b.f)
	}
	return nil
}

func (b *SliceField) BindFlag(fs *flag.FlagSet) {
	ss := NewSliceSetter(b.f.Val, b.f.Type)
	fs.Var(ss, b.f.FlagName(), b.f.Usage)
}

func (b *SliceField) Default() interface{} {
	return b.f.DefVal
}
