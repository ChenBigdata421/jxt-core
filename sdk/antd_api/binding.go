package antd_apis

import (
	"github.com/gin-gonic/gin/binding"
	"reflect"
	"strings"
	"sync"
)

const (
	_ uint8 = iota
	json
	xml
	yaml
	form
	query
)

var constructor = &bindConstructor{}

type bindConstructor struct {
	cache map[string][]uint8
	mux   sync.Mutex
}

func (e *bindConstructor) GetBindingForGin(d interface{}) []binding.Binding {
	bs := e.getBinding(reflect.TypeOf(d).String())
	if bs == nil {
		//重新构建
		bs = e.resolve(d)
	}
	gbs := make([]binding.Binding, 0)
	mp := make(map[uint8]binding.Binding, 0)
	for _, b := range bs {
		switch b {
		case json:
			mp[json] = binding.JSON
		case xml:
			mp[xml] = binding.XML
		case yaml:
			mp[yaml] = binding.YAML
		case form:
			mp[form] = binding.Form
		case query:
			mp[query] = binding.Query
		default:
			mp[0] = nil
		}
	}
	for e := range mp {
		gbs = append(gbs, mp[e])
	}
	return gbs
}

func (e *bindConstructor) resolve(d interface{}) []uint8 {
	qType := reflect.TypeOf(d)
	if qType == nil {
		return nil
	}
	// Callers pass a pointer (gin's ShouldBindWith mutates the target); unwrap it, and
	// tolerate a non-pointer struct value rather than panicking on .Elem() of a Struct.
	for qType.Kind() == reflect.Ptr {
		qType = qType.Elem()
	}
	if qType.Kind() != reflect.Struct {
		return nil
	}
	bs := make([]uint8, 0)
	var tag reflect.StructTag
	var ok bool

	for i := 0; i < qType.NumField(); i++ {
		tag = qType.Field(i).Tag
		if _, ok = tag.Lookup("json"); ok {
			bs = append(bs, json)
		}
		if _, ok = tag.Lookup("xml"); ok {
			bs = append(bs, xml)
		}
		if _, ok = tag.Lookup("yaml"); ok {
			bs = append(bs, yaml)
		}
		if _, ok = tag.Lookup("form"); ok {
			bs = append(bs, form)
		}
		if _, ok = tag.Lookup("query"); ok {
			bs = append(bs, query)
		}
		if _, ok = tag.Lookup("uri"); ok {
			bs = append(bs, 0)
		}
		if t, ok := tag.Lookup("binding"); ok && strings.Contains(t, "dive") {
			bs = append(bs, e.resolveDive(qType.Field(i).Type)...)
			continue
		}
		if t, ok := tag.Lookup("validate"); ok && strings.Contains(t, "dive") {
			bs = append(bs, e.resolveDive(qType.Field(i).Type)...)
		}
	}
	return bs
}

// resolveDive unwraps pointers/slices/arrays on a dive-tagged field's type to reach the
// underlying struct, then collects its bindings. Bindings are determined purely by struct
// tags, so a fresh pointer to a zero value of the element type is all the recursion needs —
// it never touches the caller's value, which previously broke (reflect.ValueOf(ptr).Field(i)
// is invalid on a pointer Value, and re-passing a reflect.Value made .Elem() panic).
func (e *bindConstructor) resolveDive(ft reflect.Type) []uint8 {
	for ft.Kind() == reflect.Ptr || ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array {
		ft = ft.Elem()
	}
	if ft.Kind() != reflect.Struct {
		return nil
	}
	return e.resolve(reflect.New(ft).Interface())
}

func (e *bindConstructor) getBinding(name string) []uint8 {
	e.mux.Lock()
	defer e.mux.Unlock()
	return e.cache[name]
}

func (e *bindConstructor) setBinding(name string, bs []uint8) {
	e.mux.Lock()
	defer e.mux.Unlock()
	if e.cache == nil {
		e.cache = make(map[string][]uint8)
	}
	e.cache[name] = bs
}
