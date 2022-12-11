package flagx

import (
	"flag"
	"reflect"
	"time"
)

func init() {
	IntFieldHook.Append(&Hook{(*DurationField)(nil).Fit, NewDurationField})
}

type DurationField struct {
	f      *Field
	defval time.Duration
	Max    *time.Duration
	Min    *time.Duration
}

func NewDurationField(f *Field) Fielder {
	df := &DurationField{
		f: f,
	}
	return df
}

func (d *DurationField) Init() (err error) {
	if d.f.DefVal != "" {
		d.defval, err = time.ParseDuration(d.f.DefVal)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DurationField) BindOpt(key, value string) error {
	switch key {
	case KEY_MAX, KEY_MIN:
		v, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		if key == KEY_MAX {
			d.Max = &v
		} else {
			d.Min = &v
		}
	default:
		return ErrOptMissHandler.Format(key, d.f.Name)
	}
	return nil
}

func (d *DurationField) BindFlag(fs *flag.FlagSet) {
	ins := d.f.Instance().(*time.Duration)
	fs.DurationVar(ins, d.f.FlagName(), d.defval, d.f.Usage)
}

func (d *DurationField) Fit(t reflect.Type) bool {
	return t.AssignableTo(TypeDuration)
}

func (d *DurationField) Default() interface{} {
	return d.defval
}

func (d *DurationField) AfterParse() error {
	ptr := d.f.Instance().(*time.Duration)
	if d.Min != nil {
		if *ptr < *d.Min {
			*ptr = *d.Min
		}
	}
	if d.Max != nil {
		if *ptr > *d.Max {
			*ptr = *d.Max
		}
	}
	return nil
}
