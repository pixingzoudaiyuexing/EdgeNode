//go:build script

package js

import (
	"net/http"

	"github.com/iwind/TeaGo/maps"
)

// RequestInterface 是可信 1.3.9 Plus JS Request Bridge 的精确 Go 方法集合。
type RequestInterface interface {
	Allow()
	Close()
	ContentLength() int64
	Cookie(string) string
	DeleteHeader(string)
	Done()
	Format(string) string
	Header() http.Header
	Host() string
	Id() string
	Method() string
	Node() maps.Map
	Path() string
	Proto() string
	ProtoMajor() int
	ProtoMinor() int
	RawRemoteAddr() string
	RemoteAddr() string
	RemotePort() int
	Server() maps.Map
	SetAttr(string, string)
	SetHeader(string, []string)
	SetURI(string)
	SetVar(string, string)
	TransferEncoding() string
	URI() string
	URL() string
}

// ResponseInterface 是可信 1.3.9 Plus JS Response Bridge 的精确 Go 方法集合。
type ResponseInterface interface {
	DeleteHeader(string)
	Header() http.Header
	Redirect(int, string)
	Send(int, string)
	SendFile(int, string) (int64, error)
	SendResp(*http.Response) (int64, error)
	SetHeader(string, []string)
}
