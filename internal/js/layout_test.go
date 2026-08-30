//go:build script

package js

import (
	"testing"
	"unsafe"
)

// TestCoreABILayout 固定可信 1.3.9 Plus 已确认的 amd64 字段偏移。
// HTTP Bridge 后续会依赖这些所有权关系，避免普通重构悄悄改变 ABI。
func TestCoreABILayout(t *testing.T) {
	var ctx Context
	assertOffset(t, "Context.rawContext", unsafe.Offsetof(ctx.rawContext), 0x00)
	assertOffset(t, "Context.isolate", unsafe.Offsetof(ctx.isolate), 0x08)
	assertOffset(t, "Context.objectTemplate", unsafe.Offsetof(ctx.objectTemplate), 0x10)
	assertOffset(t, "Context.goObjectID", unsafe.Offsetof(ctx.goObjectID), 0x18)
	assertOffset(t, "Context.goObjectMap", unsafe.Offsetof(ctx.goObjectMap), 0x20)
	assertOffset(t, "Context.goObjectLocker", unsafe.Offsetof(ctx.goObjectLocker), 0x28)
	assertOffset(t, "Context.serverID", unsafe.Offsetof(ctx.serverID), 0x40)
	assertOffset(t, "Context.doneCount", unsafe.Offsetof(ctx.doneCount), 0x48)

	var contextPool ContextPool
	assertOffset(t, "ContextPool.contexts", unsafe.Offsetof(contextPool.contexts), 0x00)
	assertOffset(t, "ContextPool.maxSize", unsafe.Offsetof(contextPool.maxSize), 0x08)

	var isolate Isolate
	assertOffset(t, "Isolate.rawIsolate", unsafe.Offsetof(isolate.rawIsolate), 0x00)
	assertOffset(t, "Isolate.contextPool", unsafe.Offsetof(isolate.contextPool), 0x08)
	assertOffset(t, "Isolate.uses", unsafe.Offsetof(isolate.uses), 0x10)
	assertOffset(t, "Isolate.disposed", unsafe.Offsetof(isolate.disposed), 0x14)

	var pool IsolatePool
	assertOffset(t, "IsolatePool.isolates", unsafe.Offsetof(pool.isolates), 0x00)
	assertOffset(t, "IsolatePool.dirty", unsafe.Offsetof(pool.dirty), 0x18)
	assertOffset(t, "IsolatePool.ticker", unsafe.Offsetof(pool.ticker), 0x30)
	assertOffset(t, "IsolatePool.tickIndex", unsafe.Offsetof(pool.tickIndex), 0x38)
	assertOffset(t, "IsolatePool.size", unsafe.Offsetof(pool.size), 0x40)
	assertOffset(t, "IsolatePool.current", unsafe.Offsetof(pool.current), 0x48)
}

func assertOffset(t *testing.T, field string, got uintptr, want uintptr) {
	t.Helper()
	if got != want {
		t.Fatalf("%s offset = 0x%x, want 0x%x", field, got, want)
	}
}
