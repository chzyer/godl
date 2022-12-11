package flagx

import (
	"flag"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const (
	KEY_USAGE = "usage"
	KEY_DEF   = "def"
	KEY_MAX   = "max"
	KEY_MIN   = "min"
)

var (
	ErrOptMissHandler = Error("opt(%v) in field(%v) missing handler")
	ErrOptInvalid     = Error("optString(%v) in field(%v) is invalid")
	ErrTypeSupport    = Error("type(%v) is not supported")
)

type Fielder interface {
	Init() error
	Default() interface{}
	BindFlag(*flag.FlagSet)
}

type ArgsSetter interface {
	SetArgs(v *reflect.Value, args []string) error
}

type ArgSetter interface {
	SetArg(v *reflect.Value, arg string) error
}

type OptBinder interface {
	BindOpt(key, val string) error
}

type Arg interface {
	Arg(n int) string
}

type AfterParser interface {
	AfterParse() error
}

type Field struct {
	Name     string       // field name
	Type     reflect.Type // field type
	Usage    string
	flagName string
	DefVal   string
	Val      *reflect.Value
	fielder  Fielder
}

func NewField(t reflect.StructField, val reflect.Value) (f *Field, err error) {
	f = &Field{
		Name: t.Name,
		Val:  &val,
		Type: t.Type,
	}
	if !IsPubField(t.Name) {
		return nil, nil
	}

	fieldFunc, errSelectField := f.selectFielder(t.Type)
	if errSelectField == nil {
		f.fielder = fieldFunc(f)
	}

	process, err := f.decodeTag(t.Tag)
	if errSelectField != nil && process {
		panic(errSelectField)
	} else if !process || err != nil {
		return nil, err
	}

	if err = f.fielder.Init(); err != nil {
		return nil, err
	}

	return f, nil
}

func (f *Field) selectFielder(t reflect.Type) (func(f *Field) Fielder, error) {
	switch t.Kind() {
	case reflect.Int, reflect.Int64, reflect.Uint:
		fallthrough
	case reflect.Int8, reflect.Int16, reflect.Int32:
		if n := IntFieldHook.Select(t); n != nil {
			return n, nil
		}
		return NewIntField, nil
	case reflect.Bool:
		if n := BoolFieldHook.Select(t); n != nil {
			return n, nil
		}
		return NewBoolField, nil
	case reflect.String:
		if n := StringFieldHook.Select(t); n != nil {
			return n, nil
		}
		return NewStringField, nil
	case reflect.Slice:
		if n := SliceFieldHook.Select(t); n != nil {
			return n, nil
		}
		return NewSliceField, nil
	default:
		return nil, fmt.Errorf("not support for type: %v", t)
	}
}

func (f *Field) ArgIdx() (int, bool) {
	list := RegexpArgNumName.FindAllStringSubmatch(f.flagName, -1)
	if len(list) == 0 {
		return 0, false
	}
	idx, err := strconv.Atoi(list[0][1])
	if err != nil {
		idx = -1
	}
	return idx, true
}

func (f *Field) decodeTag(t reflect.StructTag) (bool, error) {
	flagString := t.Get("flag")
	if flagString == "-" {
		return false, nil
	}
	tags := strings.Split(flagString, ";")
	start := 1

	if strings.Contains(tags[0], "=") {
		start = 0
	} else {
		f.flagName = tags[0]
	}

	for i := start; i < len(tags); i++ {
		sp := strings.Split(tags[i], "=")
		if len(sp) != 2 {
			return true, ErrOptInvalid.Format(flagString, f.Name)
		}
		switch sp[0] {
		case KEY_USAGE:
			f.Usage = sp[1]
		case KEY_DEF:
			f.DefVal = sp[1]
		default:
			ob, ok := f.fielder.(OptBinder)
			if !ok {
				return true, ErrOptMissHandler.Format(sp[0], f.Name)
			}
			if err := ob.BindOpt(sp[0], sp[1]); err != nil {
				return true, err
			}
		}
	}
	return true, nil
}

func (f *Field) FlagName() string {
	if f.flagName == "" {
		return strings.ToLower(f.Name[:1]) + f.Name[1:]
	}
	return f.flagName
}

func (f *Field) String() string {
	return fmt.Sprintf("&%+v", *f)
}

func (f *Field) Default() interface{} {
	return f.fielder.Default()
}

func (f *Field) Instance() interface{} {
	return f.Val.Addr().Interface()
}

func (f *Field) BindFlag(fs *flag.FlagSet) {
	f.fielder.BindFlag(fs)
}

func (f *Field) SetArgs(v *reflect.Value, fs *flag.FlagSet) error {
	as, ok := f.fielder.(ArgsSetter)
	if !ok {
		return fmt.Errorf("the type(%v) of field(%v) can't be args", f.Type, f.Name)
	}
	return as.SetArgs(v, fs.Args())
}

func (f *Field) SetArg(v *reflect.Value, fs *flag.FlagSet) error {
	as, ok := f.fielder.(ArgSetter)
	if !ok {
		return fmt.Errorf("field %v is not settable arg", f)
	}
	idx, ok := f.ArgIdx()
	if !ok {
		return fmt.Errorf("field %v is not define to arg", f)
	}
	return as.SetArg(v, fs.Arg(idx))
}

func (f *Field) AfterParse() error {
	ap, ok := f.fielder.(AfterParser)
	if !ok {
		return nil
	}

	return ap.AfterParse()
}
