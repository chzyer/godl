package flagx

import (
	"flag"
	"fmt"
	"reflect"
	"strconv"
	"time"
)

var (
	TypeDuration = reflect.TypeOf(time.Duration(0))
)

func chooseInt64(a, b int64, max bool) int64 {
	if a > b {
		if max {
			return a
		} else {
			return b
		}
	}
	if max {
		return b
	} else {
		return a
	}
}

type IntSetter struct {
	Val      *reflect.Value
	Kind     reflect.Kind
	Max, Min *int64
}

func NewIntSetter(val *reflect.Value, kind reflect.Kind, max, min *int64) *IntSetter {
	is := &IntSetter{
		Val:  val,
		Kind: kind,
		Max:  max,
		Min:  min,
	}
	return is
}

func (is *IntSetter) Set(s string) error {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return ErrUsage.Format(err)
	}
	is.SetInt(i)
	return nil
}

func (is *IntSetter) SetInt(i int64) {
	if is.Max != nil {
		i = chooseInt64(i, *is.Max, false)
	}
	if is.Min != nil {
		i = chooseInt64(i, *is.Min, true)
	}

	var val interface{}
	switch is.Kind {
	case reflect.Int:
		val = int(i)
	case reflect.Int8:
		val = int8(i)
	case reflect.Int16:
		val = int16(i)
	case reflect.Int32:
		val = int32(i)
	case reflect.Int64:
		val = int64(i)
	case reflect.Uint:
		val = uint(i)
	}
	is.Val.Set(reflect.ValueOf(val))
}

func (i *IntSetter) String() string {
	return fmt.Sprintf("%v", i.Val.Interface())
}

type IntField struct {
	f      *Field
	defval int64
	Max    *int64
	Min    *int64
}

func NewIntField(f *Field) Fielder {
	return &IntField{f: f}
}
func (i *IntField) Init() (err error) {
	if i.f.DefVal != "" {
		i.defval, err = strconv.ParseInt(i.f.DefVal, 10, 64)
		if err != nil {
			return err
		}
	}
	return nil
}

func (i *IntField) BindOpt(key, value string) error {
	switch key {
	case KEY_MAX, KEY_MIN:
		m, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		if key == KEY_MAX {
			i.Max = &m
		} else {
			i.Min = &m
		}
		return nil
	}
	return ErrOptMissHandler.Format(key, i.f.Name)
}

func (i *IntField) BindFlag(fs *flag.FlagSet) {
	is := NewIntSetter(i.f.Val, i.f.Type.Kind(), i.Max, i.Min)
	is.SetInt(i.defval)
	fs.Var(is, i.f.FlagName(), i.f.Usage)
}

func (i *IntField) Default() interface{} {
	return i.defval
}

func (i *IntField) SetArg(v *reflect.Value, arg string) error {
	if arg == "" {
		return nil
	}
	argInt, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return ErrUsage.Formatf("set args for field(%v) of type(%v) error", i.f.Name, i.f.Type)
	}
	is := NewIntSetter(i.f.Val, i.f.Type.Kind(), i.Max, i.Min)
	is.SetInt(argInt)
	return nil
}
