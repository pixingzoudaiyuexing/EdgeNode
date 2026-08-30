//go:build script

package js

import (
	"fmt"
	"math/big"
	"reflect"
	"strings"

	v8go "rogchap.com/v8go"
)

// NewValue 把 Library 的 Go 返回值转换为当前 Context 可使用的 V8 Value。
// nil 按原版返回 nil，让 v8go callback 自然产生 undefined。
func (c *Context) NewValue(value any) (*v8go.Value, error) {
	if value == nil { return nil, nil }
	if v, ok := value.(*v8go.Value); ok { return v, nil }
	return c.newReflectValue(reflect.ValueOf(value))
}

func (c *Context) newReflectValue(rv reflect.Value) (*v8go.Value, error) {
	if !rv.IsValid() { return nil, nil }
	if rv.Kind() == reflect.Interface || rv.Kind() == reflect.Pointer {
		if rv.IsNil() { return nil, nil }
		if v, ok := rv.Interface().(*v8go.Value); ok { return v, nil }
		return c.newReflectValue(rv.Elem())
	}

	iso := c.isolate.rawIsolate
	switch rv.Kind() {
	case reflect.String:
		return v8go.NewValue(iso, rv.String())
	case reflect.Bool:
		return v8go.NewValue(iso, rv.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		return v8go.NewValue(iso, int32(rv.Int()))
	case reflect.Int64:
		return v8go.NewValue(iso, rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return v8go.NewValue(iso, uint32(rv.Uint()))
	case reflect.Uint64:
		return v8go.NewValue(iso, rv.Uint())
	case reflect.Float32, reflect.Float64:
		return v8go.NewValue(iso, rv.Convert(reflect.TypeOf(float64(0))).Float())
	case reflect.Slice, reflect.Array:
		arrayValue, err := c.rawContext.RunScript("[]", "array.js")
		if err != nil { return nil, err }
		obj := arrayValue.Object()
		for i := 0; i < rv.Len(); i++ {
			item, err := c.newReflectValue(rv.Index(i))
			if err != nil { return nil, err }
			if item == nil { item = v8go.Undefined(iso) }
			if err := obj.SetIdx(uint32(i), item); err != nil { return nil, err }
		}
		return arrayValue, nil
	case reflect.Map:
		obj, err := c.objectTemplate.NewInstance(c)
		if err != nil { return nil, err }
		iter := rv.MapRange()
		for iter.Next() {
			key := fmt.Sprint(iter.Key().Interface())
			item, err := c.newReflectValue(iter.Value())
			if err != nil { return nil, err }
			if item == nil { item = v8go.Undefined(iso) }
			if err := obj.Set(key, item); err != nil { return nil, err }
		}
		return obj.Value, nil
	case reflect.Struct:
		if rv.Type() == reflect.TypeOf(big.Int{}) {
			b := rv.Interface().(big.Int); return v8go.NewValue(iso, &b)
		}
		obj, err := c.objectTemplate.NewInstance(c)
		if err != nil { return nil, err }
		t := rv.Type()
		for i := 0; i < rv.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" { continue }
			name := field.Name
			if tag, ok := field.Tag.Lookup("json"); ok {
				name = strings.Split(tag, ",")[0]
				if name == "-" { continue }
			}
			if name == "" && field.Name != "" { name = strings.ToLower(field.Name[:1]) + field.Name[1:] }
			item, err := c.newReflectValue(rv.Field(i)); if err != nil { return nil, err }
			if item == nil { item = v8go.Undefined(iso) }
			if err := obj.Set(name, item); err != nil { return nil, err }
		}
		return obj.Value, nil
	default:
		return nil, fmt.Errorf("unsupported JS value type %s", rv.Type())
	}
}
