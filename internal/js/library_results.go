//go:build script

package js

import "errors"

var (
	errJSGoObjectNotFound    = errors.New("go object not found")
	errJSHeaderNameEmpty     = errors.New("header name should not be empty")
	errJSRedirectURLRequired = errors.New("redirect() function require 'url' parameter")
)

// JSGoObjectNotFound 与 JSSuccess 是原版所有 JS Library 共享的两种标准返回。
func (l *JSBaseLibrary) JSGoObjectNotFound() (any, error) { return nil, errJSGoObjectNotFound }
func (l *JSBaseLibrary) JSSuccess() (any, error)          { return nil, nil }
