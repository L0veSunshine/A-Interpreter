package object

func NewEnvironment() *Environment {
	s := map[string]Object{}
	env := &Environment{store: s}
	return env
}

type Environment struct {
	store map[string]Object
	next  *Environment
}

func NewEnclosedEnvironment(outer *Environment) *Environment {
	env := NewEnvironment()
	env.next = outer
	return env
}

func (e *Environment) Get(name string) (Object, bool) {
	var obj Object = nil
	var ok = false
	obj, ok = e.store[name]
	if !ok && e.next != nil {
		obj, ok = e.next.Get(name)
	}
	return obj, ok
}

func (e *Environment) Set(name string, val Object) Object {
	e.store[name] = val
	return val
}
