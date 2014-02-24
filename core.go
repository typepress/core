/*
  core 提供全局可用变量, 方法.
  全局可用的类型定义请
	import "github.com/typepress/core/types"
*/
package core

import (
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strings"

	. "github.com/typepress/core/types"

	"github.com/achun/tom-toml"
	"github.com/codegangsta/martini"
	"github.com/typepress/accessflags"
	"github.com/typepress/db"
	"github.com/typepress/log"
)

var (
	// global
	Conf       toml.Toml
	Log        log.Loggers
	Db         db.Database
	PWD        string
	safeRouter martini.Router // All method, Router.NotFound(NotFound(), SubAny.Handle) already.
	SubGet     martini.Router // GET method only
	SubPut     martini.Router // PUT method only
	SubHead    martini.Router // HEAD method only
	SubPost    martini.Router // POST method only
	SubAjax    martini.Router // POST method only, with head: X-Requested-With "XMLHttpRequest"
	SubPatch   martini.Router // PATCH method only
	SubDelete  martini.Router // DELETE method only
	SubOptions martini.Router // OPTIONS method only
	SubAny     martini.Router // Any method
)

const (
	SessionName    = "TypePression"
	ServerShutDown = "server shutdown"
)

// 默认的 Martini 对象
var safeMartini = martini.New()

// 临时保存 safeMartini 的 handlers
var cacheHandlers = []martini.Handler{}

var started bool

func appStart() bool {
	return started
}

// 给默认 Martini 对象添加 handler
func Handler(handler ...martini.Handler) {
	if !appStart() {
		cacheHandlers = append(cacheHandlers, handler...)
	}
}

/*
  返回内置的 Martini 对象, 只能调用一次, 再次调用返回 nil.
  参数 handler 会优先于通过 Handler 添加的 handler 执行.
  已经执行过 .Action(Router.Handle)
*/
func Martini(handler ...martini.Handler) (*martini.Martini, martini.Router) {
	if appStart() {
		return nil, nil
	}
	started = true
	safeMartini.Handlers(append(handler, cacheHandlers...)...)
	safeMartini.Action(safeRouter.Handle)
	callInit()
	return safeMartini, safeRouter
}

var notifyMaps map[string][]int
var notifyFn []func(os.Signal) bool

// ListenSignal 增加监听 sigs 的函数.
// 监听函数 fn 的返回值如果是 true, 表示触发后剔除此监听函数.
func ListenSignal(fn func(os.Signal) bool, sigs ...os.Signal) {
	if appStart() {
		return
	}
	waitSigs := []os.Signal{}
	for _, sig := range sigs {
		key := sig.String()
		_, ok := notifyMaps[key]
		if !ok {
			notifyMaps[key] = []int{}
			_, ok := sig.(*StringSignal)
			if !ok {
				waitSigs = append(waitSigs, sig)
			}
		}

		i := len(notifyFn)
		notifyMaps[key] = append(notifyMaps[key], i)
		notifyFn = append(notifyFn, fn)
	}
	if len(waitSigs) != 0 {
		go signalNotify(waitSigs)
	}
}

// 按照 LIFO 的次序调用通过 Listen 增加的监听函数.
// 如果捕获到 panic 中断调用, 并且监听函数会被剔除.
func FireSignal(sig os.Signal) {
	idx := notifyMaps[sig.String()]
	for i := len(idx); i > 0; {
		i--
		if i >= len(notifyFn) {
			continue
		}
		fn := notifyFn[i]
		if fn == nil {
			continue
		}
		var clear bool
		err := Recover(func() { clear = fn(sig) })
		if clear || err != nil {
			notifyFn[i] = nil
		}
		if err != nil {
			break
		}
	}
}

func signalNotify(sigs []os.Signal) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sigs...)
	for {
		FireSignal(<-ch)
	}
}

func Recover(fn func()) (err interface{}) {
	defer func() {
		err = recover()
	}()
	fn()
	return
}

func init() {
	var err error
	PWD, err = os.Getwd()
	if err != nil {
		panic(err)
	}
	Log = log.Multi().(log.Loggers)
	notifyMaps = map[string][]int{}

	safeRouter = martini.NewRouter()
	SubGet = martini.NewRouter()
	SubPut = martini.NewRouter()
	SubHead = martini.NewRouter()
	SubPost = martini.NewRouter()
	SubAjax = martini.NewRouter()
	SubPatch = martini.NewRouter()
	SubDelete = martini.NewRouter()
	SubOptions = martini.NewRouter()
	SubAny = martini.NewRouter()
	safeRouter.NotFound(notFound, SubAny.Handle)
}

// NotFound Handler for Router, auto invoke other router by req.Method
func NotFound() martini.Handler {
	return notFound
}

func notFound(res http.ResponseWriter, req *http.Request, c martini.Context) {
	switch req.Method {
	case "GET":
		SubGet.Handle(res, req, c)
	case "PUT":
		SubPut.Handle(res, req, c)
	case "HEAD":
		SubHead.Handle(res, req, c)
	case "POST":
		if req.Header.Get("X-Requested-With") == "XMLHttpRequest" {
			SubAjax.Handle(res, req, c)
		} else {
			SubPost.Handle(res, req, c)
		}

	case "PATCH":
		SubPatch.Handle(res, req, c)
	case "DELETE":
		SubDelete.Handle(res, req, c)
	case "OPTIONS":
		SubOptions.Handle(res, req, c)
	}
}

var initfn []func()

func callInit() {
	for _, f := range initfn {
		f()
	}
}

// 注册初始化函数,fn 会在 Martini() 被调用的时候执行.
func RegisterInit(fn ...func()) {
	if appStart() {
		return
	}
	initfn = append(initfn, fn...)
}

var routerMap map[string]martini.Router

func init() {
	routerMap = map[string]martini.Router{
		"GET":     SubGet,
		"PUT":     SubPut,
		"HEAD":    SubHead,
		"POST":    SubPost,
		"AJAX":    SubAjax,
		"PATCH":   SubPatch,
		"DELETE":  SubDelete,
		"DEL":     SubDelete,
		"OPTIONS": SubOptions,
		"OPT":     SubOptions,
		"ANY":     SubAny,
	}
}

// github.com/achun/testing-want_test.ExamplePanic
// github.com/achun/testing-want/GET%2eUd%2edd.ExamplePanic

// 自动注册路由, 不支持本地包, main 包.
// 目前支持来自 github.com 的 package
func AutoRouter(pattern string, h ...martini.Handler) {
	const GITHUB = "github.com"
	if appStart() {
		return
	}
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		return
	}
	name := runtime.FuncForPC(pc).Name()
	names := strings.Split(name, "/")
	if len(names) < 4 || names[0] != GITHUB {
		println("AutoRouter not support:", name)
		os.Exit(1)
		return
	}
	names = names[3:]
	l := len(names) - 1
	pos := strings.LastIndex(names[l], ".")
	if pos != -1 {
		names[l] = names[l][:pos]
	}
	names[l] = strings.Replace(names[l], `%2e`, `.`, -1)
	names = append(names[:l], strings.Split(names[l], `.`)...)
	// fetch role,method
	var roles, methods []string
	for l >= 0 {
		name = names[l]
		if name == strings.ToUpper(name) {
			methods = append(methods, name)
		} else if name != strings.ToLower(name) {
			roles = append(roles, strings.ToLower(name))
		} else {
			l--
			continue
		}
		names = append(names[:l], names[l+1:]...)
		l--
	}

	pattern = "/" + strings.Join(names, "/") + pattern
	if len(roles) != 0 {
		h = append([]martini.Handler{RBAC(roles)}, h...)
	}

	if len(methods) == 0 {
		SubAny.Any(pattern, h...)
		return
	}

	for _, method := range methods {
		r := routerMap[method]
		if r == nil {
			SubAny.Any(pattern, h...)
		} else {
			r.Any(pattern, h...)
		}
	}
}

var rolesAll = []string{}

// 设置角色名称集合,用于角色控制.
// 如果要启用角色控制, 必须在注册路由之前进行设置.
// 角色值会被转化为小写, 排序
func RolesSet(rs ...string) {
	if appStart() {
		return
	}
	rolesAll = filpSlice(append(rolesAll, rs...))
}

func filpSlice(a []string) []string {
	l := len(a)
	if l <= 1 {
		return a
	}

	sort.Sort(sort.StringSlice(a))
	s := 0
	i := 1
	for i < l {
		if a[i] != a[s] {
			s++
			a[s] = a[i]
		}
		i++
	}
	if s > 63 {
		s = 63
	}
	return a[:s+1]
}

// 根据角色名称计算角色数字值.
func rolesID(rs []string) (x Role) {
	rs = filpSlice(rs)
	l := len(rolesAll)
	for _, s := range rs {
		i := sort.SearchStrings(rolesAll, s)
		if i < l && s == rolesAll[i] {
			x = x | 1<<uint(i)
		}
	}
	return x
}

// role-based access control
func RBAC(rs []string) martini.Handler {
	return accessflags.Forbidden(rolesID(rs))
}
