package types

import (
	"net/http"
	"os"
)

// +dl en
// i18n translator interface.
// +dl

// i18n 翻译器接口.
type Translator interface {
	Sprint(v ...interface{}) string
	Sprintf(format string, v ...interface{}) string
	Source(src string)
}

// +dl en
// string os.Signal
// +dl

// 字符串信号, 实现 os.Signal 接口
type StringSignal struct {
	Str string
	X   interface{}
}

// +dl en
// NewStringSignal returns an os.Signal.
// +dl

// NewStringSignal 返回字符串 os.Signal 信号.
func NewStringSignal(str string, x interface{}) os.Signal {
	return &StringSignal{str, x}
}

func (ss *StringSignal) Signal() {}
func (ss *StringSignal) String() string {
	return ss.Str
}

// +dl en
// For Role-based access control
// +dl

// Role 用于角色控制, 参见 github.com/typepress/accessflags
type Role uint64

type ContentDir http.Dir
type TemplateDir http.Dir
