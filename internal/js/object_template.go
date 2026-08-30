//go:build script

package js

import v8go "rogchap.com/v8go"

// ObjectTemplate 保留 Context 所属关系，并包装原始 v8go ObjectTemplate。
type ObjectTemplate struct {
	ctx *Context
	raw *v8go.ObjectTemplate
}

func (t *ObjectTemplate) NewInstance(ctx *Context) (*v8go.Object, error) {
	return t.raw.NewInstance(ctx.rawContext)
}

func (t *ObjectTemplate) SetInternalFieldCount(count uint32) {
	t.raw.SetInternalFieldCount(count)
}
