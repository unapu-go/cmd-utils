package cmdu

import (
	"fmt"
	"os"
	"sort"

	"mvdan.cc/sh/v3/expand"
)

type UniquerEnviron interface {
	expand.Environ

	Unique(f func(name string, vr expand.Variable) bool)
}

type Environ struct {
	parent expand.Environ
	values map[string]expand.Variable
}

func NewEnviron(parent expand.Environ, values map[string]expand.Variable) *Environ {
	if values == nil {
		values = map[string]expand.Variable{}
	}
	return &Environ{parent: parent, values: values}
}

func OsEnv() *Environ {
	return NewEnviron(expand.ListEnviron(os.Environ()...), nil)
}

func (o *Environ) Get(name string) expand.Variable {
	if vr, ok := o.values[name]; ok {
		return vr
	}
	if o.parent != nil {
		return o.parent.Get(name)
	}
	return expand.Variable{}
}

func (o *Environ) SetString(name, value string) (err error) {
	return o.Set(name, expand.Variable{Str: value, Kind: expand.String})
}

func (o *Environ) Set(name string, vr expand.Variable) error {
	prev := o.Get(name)
	if o.values == nil {
		o.values = make(map[string]expand.Variable)
	}
	if !vr.IsSet() && (vr.Exported || vr.Local || vr.ReadOnly) {
		// marking as exported/local/readonly
		prev.Exported = prev.Exported || vr.Exported
		prev.Local = prev.Local || vr.Local
		prev.ReadOnly = prev.ReadOnly || vr.ReadOnly
		vr = prev
		o.values[name] = vr
		return nil
	}
	if prev.ReadOnly {
		return fmt.Errorf("readonly variable")
	}
	if !vr.IsSet() { // unsetting
		if prev.Local {
			vr.Local = true
			o.values[name] = vr
			return nil
		}
		delete(o.values, name)
		if writeEnv, _ := o.parent.(expand.WriteEnviron); writeEnv != nil {
			writeEnv.Set(name, vr)
			return nil
		}
	}
	// modifying the entire variable
	vr.Local = prev.Local || vr.Local
	o.values[name] = vr
	return nil
}

func (o *Environ) Each(f func(name string, vr expand.Variable) bool) {
	if o.parent != nil {
		o.parent.Each(f)
	}
	for name, vr := range o.values {
		if !f(name, vr) {
			return
		}
	}
}

func EnvMap(env expand.Environ) (res map[string]expand.Variable) {
	res = map[string]expand.Variable{}

	env.Each(func(name string, vr expand.Variable) bool {
		res[name] = vr
		return true
	})

	return res
}

func EnvStrings(env expand.Environ, f func(v *expand.Variable) bool) (res []string) {
	if f == nil {
		f = func(v *expand.Variable) bool {
			return true
		}
	}
	envm := EnvMap(env)
	for name, vr := range envm {
		if f(&vr) {
			res = append(res, fmt.Sprintf("%s=%s", name, vr))
		}
	}
	sort.Strings(res)
	return
}
