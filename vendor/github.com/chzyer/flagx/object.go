package flagx

import (
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
)

var (
	RegexpArgNumName = regexp.MustCompile(`^\[(\d*)\]$`)
	ErrUsage         = Error("%v")
	CmdFlagConfig    = &FlagConfig{
		Name:          os.Args[0],
		ErrorHandling: flag.ExitOnError,
		Args:          os.Args[1:],
	}
)

type Object struct {
	Type     reflect.Type
	Val      reflect.Value
	Opt      []*Field
	Arg      []*Field
	Usage    func()
	isAllArg bool
}

func NewObject(obj interface{}) (*Object, error) {
	v := reflect.ValueOf(obj)
	t := v.Type()
	if t.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("obj must be a struct pointer")
	}
	t = t.Elem()
	v = v.Elem()
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("obj must be a struct pointer")
	}

	o := &Object{
		Type: t,
		Val:  v,
	}

	if err := o.parseFields(t, &v); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *Object) parseFields(t reflect.Type, v *reflect.Value) error {
	argValidate := make([]bool, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field, err := NewField(t.Field(i), v.Field(i))
		if err != nil {
			return err
		}
		if field == nil {
			continue
		}
		if idx, ok := field.ArgIdx(); ok {
			if idx == -1 {
				if o.isAllArg {
					return fmt.Errorf("only can have one [] arg")
				}
				if len(o.Arg) > 0 {
					return fmt.Errorf("only can use [] arg if only have one specified [] arg")
				}
				o.isAllArg = true
			} else if o.isAllArg {
				return fmt.Errorf("can't use [\\d] if have one [] arg")
			}
			if idx >= len(argValidate) {
				return fmt.Errorf("invalid arg index %d", idx)
			}
			if !o.isAllArg {
				argValidate[idx] = true
			}
			o.Arg = append(o.Arg, field)
		} else {
			o.Opt = append(o.Opt, field)
		}
	}

	if !o.isAllArg {
		for idx := range o.Arg {
			if !argValidate[idx] {
				return fmt.Errorf("missing arg idx: %d", idx)
			}
		}
	}

	return nil
}

func (o *Object) usage(fs *flag.FlagSet, name string) {
	arg := ""
	if len(o.Opt) > 0 {
		arg += "[option] "
	}
	for _, f := range o.Arg {
		idx, _ := f.ArgIdx()
		arg += "<" + f.Name
		if idx < 0 {
			arg += "..."
		}
		arg += "> "
	}

	io.WriteString(os.Stderr, fmt.Sprintf("%s %s\n", name, arg))
	if len(o.Opt) > 0 {
		fmt.Fprintf(os.Stderr, "\noption:\n")
		fs.VisitAll(func(f *flag.Flag) {
			format := "  -%s=%s"
			fmt.Fprintf(os.Stderr, format, f.Name, f.DefValue)
			if f.Usage != "" {
				fmt.Fprintf(os.Stderr, ": %s", f.Usage)
			}
			fmt.Fprintln(os.Stderr)
		})
	}
}

func (o *Object) Parse() error {
	return o.ParseFlag(CmdFlagConfig)
}

type FlagConfig struct {
	Name          string
	ErrorHandling flag.ErrorHandling
	Args          []string
}

func (o *Object) ParseFlag(fc *FlagConfig) error {
	fs := flag.NewFlagSet(fc.Name, fc.ErrorHandling)
	for _, f := range o.Opt {
		f.BindFlag(fs)
	}
	fs.Usage = func() {
		o.usage(fs, fc.Name)
	}
	o.Usage = fs.Usage

	if err := fs.Parse(fc.Args); err != nil {
		return err
	}

	for _, f := range o.Opt {
		if err := f.AfterParse(); err != nil {
			return err
		}
	}

	for _, f := range o.Arg {
		idx, _ := f.ArgIdx()
		if idx < 0 {
			if err := f.SetArgs(f.Val, fs); err != nil {
				return err
			}
		} else {
			if err := f.SetArg(f.Val, fs); err != nil {
				return err
			}
		}
	}

	return nil
}
