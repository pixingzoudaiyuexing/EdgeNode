//go:build script

package js

import "sync"

// Library 是 1.3.9 Plus JS 内建库的生命周期接口。
// Scripts-1 只恢复通用生命周期；具体 HTTP、URL、Redis 等库在 Scripts-2 分批恢复。
type Library interface {
	JSDispose(ctx *Context)
	JSDone(ctx *Context)
	JSInit(ctx *Context)
	JSNamespace() string
	JSPrototype() string
}

// JSBaseLibrary 提供原版无副作用的基础生命周期实现。
type JSBaseLibrary struct{}

func (l *JSBaseLibrary) JSInit(ctx *Context)    {}
func (l *JSBaseLibrary) JSDone(ctx *Context)    {}
func (l *JSBaseLibrary) JSDispose(ctx *Context) {}

// LibraryManager 保存按注册顺序执行的 JS Library。
// 字段顺序与 1.3.9 Plus 静态 ABI 保持一致：slice 后紧跟 RWMutex。
type LibraryManager struct {
	libraries []Library
	locker    sync.RWMutex
}

var SharedLibraryManager = &LibraryManager{}

func (m *LibraryManager) Register(library Library) {
	m.locker.Lock()
	m.libraries = append(m.libraries, library)
	m.locker.Unlock()
}

func (m *LibraryManager) All() []Library {
	m.locker.RLock()
	libraries := m.libraries
	m.locker.RUnlock()
	return libraries
}
