package types

import (
	"os"
)

type Translator interface {
	Sprint(v ...interface{}) string
	Sprintf(format string, v ...interface{}) string
	Source(src string)
}

// 字符串信号
type StringSignal struct {
	Str string
	X   interface{}
}

func NewStringSignal(str string, x interface{}) os.Signal {
	return &StringSignal{str, x}
}

func (ss *StringSignal) Signal() {}
func (ss *StringSignal) String() string {
	return ss.Str
}

type Role uint64
