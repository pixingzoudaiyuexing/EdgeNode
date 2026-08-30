//go:build script

package js

import (
	"github.com/iwind/TeaGo/types"
	v8go "rogchap.com/v8go"
)

// FunctionArguments 包装一次从 JavaScript 进入 Go 的函数调用。
// 字段顺序与可信 1.3.9 Plus ABI 保持一致：Context 在前，V8 callback info 在后。
type FunctionArguments struct {
	ctx  *Context
	info *v8go.FunctionCallbackInfo
}

func (a *FunctionArguments) Context() *Context { return a.ctx }
func (a *FunctionArguments) Args() []*v8go.Value { return a.info.Args() }
func (a *FunctionArguments) This() *v8go.Object { return a.info.This() }

func (a *FunctionArguments) ArgAt(index int) (*v8go.Value, bool) {
	args := a.info.Args()
	if index < 0 || index >= len(args) {
		return nil, false
	}
	return args[index], true
}

func (a *FunctionArguments) StringAt(index int) (string, bool) {
	v, ok := a.ArgAt(index)
	if !ok || !v.IsString() {
		return "", false
	}
	return v.String(), true
}

// FormatStringAt 与原版一致：字符串和数字都按 JavaScript String(value) 格式化。
func (a *FunctionArguments) FormatStringAt(index int) (string, bool) {
	v, ok := a.ArgAt(index)
	if !ok || (!v.IsString() && !v.IsNumber()) {
		return "", false
	}
	return v.String(), true
}

func (a *FunctionArguments) StringsAt(index int) ([]string, bool) {
	v, ok := a.ArgAt(index)
	if !ok || !v.IsArray() {
		return nil, false
	}
	obj := v.Object()
	lengthValue, err := obj.Get("length")
	if err != nil || !lengthValue.IsUint32() {
		return nil, false
	}
	length := lengthValue.Uint32()
	result := make([]string, 0, length)
	for i := uint32(0); i < length; i++ {
		item, err := obj.GetIdx(i)
		if err != nil || !item.IsString() {
			return nil, false
		}
		result = append(result, item.String())
	}
	return result, true
}

// FormatStringsAt 接受 JS Array；非字符串/数字元素按原版直接跳过，而不是让整个转换失败。
func (a *FunctionArguments) FormatStringsAt(index int) ([]string, bool) {
	v, ok := a.ArgAt(index)
	if !ok || !v.IsArray() {
		return nil, false
	}
	obj := v.Object()
	lengthValue, err := obj.Get("length")
	if err != nil || !lengthValue.IsUint32() {
		return nil, false
	}
	length := lengthValue.Uint32()
	result := make([]string, 0, length)
	for i := uint32(0); i < length; i++ {
		item, err := obj.GetIdx(i)
		if err != nil {
			continue
		}
		if item.IsString() || item.IsNumber() {
			result = append(result, item.String())
		}
	}
	return result, true
}

func (a *FunctionArguments) ObjectAt(index int) (*v8go.Object, bool) {
	v, ok := a.ArgAt(index)
	if !ok || !v.IsObject() {
		return nil, false
	}
	return v.Object(), true
}

func (a *FunctionArguments) GoObjectAt(index int) (any, bool) {
	v, ok := a.ArgAt(index)
	if !ok || !v.IsUint32() {
		return nil, false
	}
	object := a.ctx.GoObject(v.Uint32())
	return object, object != nil
}

func (a *FunctionArguments) IntAt(index int) (int, bool) {
	v, ok := a.ArgAt(index)
	if !ok {
		return 0, false
	}
	switch {
	case v.IsInt32():
		return int(v.Int32()), true
	case v.IsUint32():
		return int(v.Uint32()), true
	case v.IsBigInt():
		b := v.BigInt()
		if b == nil || !b.IsInt64() { return 0, false }
		return int(b.Int64()), true
	case v.IsNumber():
		return int(v.Number()), true
	case v.IsString():
		return types.Int(v.String()), true
	default:
		return 0, false
	}
}

func (a *FunctionArguments) Int64At(index int) (int64, bool) {
	v, ok := a.ArgAt(index)
	if !ok { return 0, false }
	switch {
	case v.IsInt32(): return int64(v.Int32()), true
	case v.IsUint32(): return int64(v.Uint32()), true
	case v.IsBigInt():
		b := v.BigInt(); if b == nil || !b.IsInt64() { return 0, false }; return b.Int64(), true
	case v.IsNumber(): return int64(v.Number()), true
	case v.IsString(): return types.Int64(v.String()), true
	default: return 0, false
	}
}

func (a *FunctionArguments) Float32At(index int) (float32, bool) {
	v, ok := a.ArgAt(index); if !ok { return 0, false }
	switch {
	case v.IsInt32(): return float32(v.Int32()), true
	case v.IsUint32(): return float32(v.Uint32()), true
	case v.IsBigInt(): b:=v.BigInt(); if b==nil || !b.IsInt64(){return 0,false}; return float32(b.Int64()),true
	case v.IsNumber(): return float32(v.Number()), true
	case v.IsString(): return types.Float32(v.String()), true
	default: return 0,false
	}
}

func (a *FunctionArguments) Float64At(index int) (float64, bool) {
	v, ok := a.ArgAt(index); if !ok { return 0, false }
	switch {
	case v.IsInt32(): return float64(v.Int32()), true
	case v.IsUint32(): return float64(v.Uint32()), true
	case v.IsBigInt(): b:=v.BigInt(); if b==nil || !b.IsInt64(){return 0,false}; return float64(b.Int64()),true
	case v.IsNumber(): return v.Number(), true
	case v.IsString(): return types.Float64(v.String()), true
	default: return 0,false
	}
}

// BytesAt 只接受 ArrayBuffer；普通 JS Array 不在该 ABI 内。
func (a *FunctionArguments) BytesAt(index int) ([]byte, bool) {
	v, ok := a.ArgAt(index)
	if !ok || !v.IsArrayBuffer() { return nil, false }
	obj := v.Object()
	lengthValue, err := obj.Get("byteLength")
	if err != nil || !lengthValue.IsUint32() { return nil, false }
	length := lengthValue.Uint32()
	result := make([]byte, length)
	for i := uint32(0); i < length; i++ {
		item, err := obj.GetIdx(i)
		if err != nil { return nil, false }
		result[i] = byte(item.Uint32())
	}
	return result, true
}
