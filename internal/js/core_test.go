//go:build script

package js

import (
	"testing"
	"time"
)

func TestContextGoObjectResetAndReuse(t *testing.T) {
	isolate, err := NewIsolateWithContexts(1)
	if err != nil {
		t.Fatal(err)
	}
	defer isolate.Dispose()

	ctx, err := isolate.GetContext()
	if err != nil {
		t.Fatal(err)
	}
	id1 := ctx.AddGoObject("first")
	id2 := ctx.AddGoObject("second")
	if id1 != 1 || id2 != 2 {
		t.Fatalf("ids = %d,%d, want 1,2", id1, id2)
	}
	if got := ctx.GoObject(id2); got != "second" {
		t.Fatalf("GoObject(%d) = %#v", id2, got)
	}

	isolate.PutContext(ctx)
	ctx, err = isolate.GetContext()
	if err != nil {
		t.Fatal(err)
	}
	defer isolate.PutContext(ctx)
	if got := ctx.GoObject(id1); got != nil {
		t.Fatalf("old go object leaked: %#v", got)
	}
	if id := ctx.AddGoObject("again"); id != 1 {
		t.Fatalf("id after Done = %d, want 1", id)
	}
}

func TestBootstrapRunOnce(t *testing.T) {
	isolate, err := NewIsolateWithContexts(1)
	if err != nil {
		t.Fatal(err)
	}
	defer isolate.Dispose()

	ctx, err := isolate.GetContext()
	if err != nil {
		t.Fatal(err)
	}
	defer isolate.PutContext(ctx)

	value, err := ctx.RunScript(`
		let n = 0;
		gojs.once(function () { n += 1; });
		gojs.once(function () { n += 2; });
		gojs.runOnce();
		gojs.runOnce();
		n;
	`, "bootstrap_test.js")
	if err != nil {
		t.Fatal(err)
	}
	if got := value.Int32(); got != 3 {
		t.Fatalf("runOnce result = %d, want 3", got)
	}
}

func TestIsolateOverUsesBoundary(t *testing.T) {
	isolate, err := NewIsolateWithContexts(1)
	if err != nil {
		t.Fatal(err)
	}
	defer isolate.Dispose()
	isolate.uses = 4096
	if isolate.OverUses() {
		t.Fatal("4096 uses must not be overused")
	}
	isolate.uses = 4097
	if !isolate.OverUses() {
		t.Fatal("4097 uses must be overused")
	}
}

func TestIsolatePoolDirtyWaitsForContextReturn(t *testing.T) {
	pool, err := NewIsolatePool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	old := pool.isolates[0]
	ctx, err := pool.GetContext()
	if err != nil {
		t.Fatal(err)
	}
	old.uses = 4097
	pool.tick()
	if pool.isolates[0] == old {
		t.Fatal("overused isolate was not replaced")
	}
	if len(pool.dirty) != 1 || pool.dirty[0] != old {
		t.Fatal("old isolate was not appended to dirty queue")
	}
	if old.disposed {
		t.Fatal("in-use dirty isolate disposed too early")
	}

	pool.PutContext(ctx)
	pool.tick()
	if !old.disposed {
		t.Fatal("dirty isolate was not disposed after context return")
	}
	if len(pool.dirty) != 0 {
		t.Fatalf("dirty queue size = %d, want 0", len(pool.dirty))
	}

	pool.isolates[0].Dispose()
}

type lifecycleLibrary struct {
	JSBaseLibrary
	initCount    int
	doneCount    int
	disposeCount int
}

func (l *lifecycleLibrary) JSNamespace() string { return "gojs.test" }
// 该测试只验证 Library 生命周期；真实 loader 会执行 JSPrototype，因此这里返回空代码而不是旧占位符 "Test"。
func (l *lifecycleLibrary) JSPrototype() string { return "" }
func (l *lifecycleLibrary) JSInit(ctx *Context)  { l.initCount++ }
func (l *lifecycleLibrary) JSDone(ctx *Context)  { l.doneCount++ }
func (l *lifecycleLibrary) JSDispose(ctx *Context) {
	l.disposeCount++
}

func TestLibraryLifecycleAndDoubleDone(t *testing.T) {
	original := SharedLibraryManager
	manager := &LibraryManager{}
	library := &lifecycleLibrary{}
	manager.Register(library)
	SharedLibraryManager = manager
	defer func() { SharedLibraryManager = original }()

	isolate, err := NewIsolateWithContexts(1)
	if err != nil {
		t.Fatal(err)
	}
	if library.initCount != 1 {
		t.Fatalf("JSInit count = %d, want 1", library.initCount)
	}
	// 初始化 Context 入池时 ContextPool.Put 会调用一次 Done。
	if library.doneCount != 1 {
		t.Fatalf("initial JSDone count = %d, want 1", library.doneCount)
	}

	ctx, err := isolate.GetContext()
	if err != nil {
		isolate.Dispose()
		t.Fatal(err)
	}
	isolate.PutContext(ctx)
	// Isolate.PutContext + ContextPool.Put 按原版连续调用两次 Done。
	if library.doneCount != 3 {
		isolate.Dispose()
		t.Fatalf("JSDone count after PutContext = %d, want 3", library.doneCount)
	}
	isolate.Dispose()
	if library.disposeCount != 1 {
		t.Fatalf("JSDispose count = %d, want 1", library.disposeCount)
	}
}

func TestIsolatePoolTickerInterval(t *testing.T) {
	pool, err := NewIsolatePool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if pool.ticker == nil {
		t.Fatal("ticker is nil")
	}
	// time.Ticker 不暴露 Duration；这里确认一次 tick 不会在明显短于原版 5 秒的时间内到达。
	select {
	case <-pool.ticker.C:
		t.Fatal("ticker fired too early")
	case <-time.After(25 * time.Millisecond):
	}
}
