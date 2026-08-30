//go:build script

package js

import (
	"reflect"
	"testing"
)

// 固定可信 1.3.9 Plus 恢复出来的 HTTP Bridge 方法集合，避免后续维护时
// 只改接口或 Library 一侧而静默丢失 JavaScript 能力。
func TestHTTPBridgeMethodCoverage(t *testing.T) {
	assertLibraryCoversInterface(t, reflect.TypeOf((*RequestInterface)(nil)).Elem(), reflect.TypeOf(&JSNetHTTPRequestLibrary{}))
	assertLibraryCoversInterface(t, reflect.TypeOf((*ResponseInterface)(nil)).Elem(), reflect.TypeOf(&JSNetHTTPResponseLibrary{}))
}

func assertLibraryCoversInterface(t *testing.T, interfaceType reflect.Type, libraryType reflect.Type) {
	t.Helper()
	for i := 0; i < interfaceType.NumMethod(); i++ {
		name := interfaceType.Method(i).Name
		if _, ok := libraryType.MethodByName(name); !ok {
			t.Fatalf("%s 缺少原版 Bridge 方法 %s", libraryType, name)
		}
	}
}
