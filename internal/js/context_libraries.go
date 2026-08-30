//go:build script

package js

import (
	"fmt"
	"reflect"
	"strings"

	v8go "rogchap.com/v8go"
)

func (c *Context) installLibrary(library Library) error {
	library.JSInit(c)
	namespace := library.JSNamespace()
	if !strings.HasPrefix(namespace, "gojs.") {
		return fmt.Errorf("JS namespace %q must be started with prefix 'gojs.'", namespace)
	}
	if _, err := c.rawContext.RunScript(fmt.Sprintf("gojs.prepareNamespace(%q)", namespace), "utils.js"); err != nil {
		return err
	}

	prototype := strings.ReplaceAll(library.JSPrototype(), "$this.", namespace+".")
	if prototype != "" {
		if _, err := c.rawContext.RunScript(prototype, namespace+".js"); err != nil { return err }
	}

	namespaceValue, err := c.rawContext.RunScript(namespace, namespace+".js")
	if err != nil { return err }
	namespaceObject := namespaceValue.Object()

	libraryValue := reflect.ValueOf(library)
	libraryType := libraryValue.Type()
	for i := 0; i < libraryValue.NumMethod(); i++ {
		methodInfo := libraryType.Method(i)
		if strings.HasPrefix(methodInfo.Name, "JS") { continue }
		method := libraryValue.Method(i)
		if method.Type().NumIn() != 1 || method.Type().In(0) != reflect.TypeOf((*FunctionArguments)(nil)) || method.Type().NumOut() < 2 {
			continue
		}

		functionTemplate := v8go.NewFunctionTemplate(c.isolate.rawIsolate, func(info *v8go.FunctionCallbackInfo) *v8go.Value {
			results := method.Call([]reflect.Value{reflect.ValueOf(&FunctionArguments{ctx: c, info: info})})
			if len(results) < 2 { return c.throwJS(fmt.Errorf("invalid JS library method result")) }
			if !results[1].IsNil() {
				if err, ok := results[1].Interface().(error); ok { return c.throwJS(err) }
				return c.throwJS(fmt.Errorf("invalid JS library error result"))
			}
			value, err := c.NewValue(results[0].Interface())
			if err != nil { return c.throwJS(err) }
			return value
		})
		if err := namespaceObject.Set(methodInfo.Name, functionTemplate.GetFunction(c.rawContext)); err != nil { return err }
	}
	return nil
}

func (c *Context) throwJS(err error) *v8go.Value {
	value, valueErr := v8go.NewValue(c.isolate.rawIsolate, err.Error())
	if valueErr != nil { return nil }
	return c.isolate.rawIsolate.ThrowException(value)
}
