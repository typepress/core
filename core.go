/*
  core 提供全局可用变量, 方法.
  全局可用的类型定义请
	import "github.com/typepress/types"
*/
package core

import (
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"

	"github.com/achun/tom-toml"
	"github.com/go-martini/martini"

	. "github.com/typepress/types"

	"github.com/typepress/accessflags"
	"github.com/typepress/db"
	"github.com/typepress/log"
)

var (
	// global
	Conf          toml.Toml
	Log           log.Loggers
	Db            db.Database
	PWD           string
	safeRouter    martini.Router // All method,
	RouterGet     martini.Router // GET method only
	RouterPut     martini.Router // PUT method only
	RouterHead    martini.Router // HEAD method only
	RouterPost    martini.Router // POST method only
	RouterAjax    martini.Router // POST method only, with head: X-Requested-With "XMLHttpRequest"
	RouterPatch   martini.Router // PATCH method only
	RouterDelete  martini.Router // DELETE method only
	RouterOptions martini.Router // OPTIONS method only
	RouterAny     martini.Router // Any method
)

const (
	SessionName    = "TPSession"
	ServerShutDown = "ServerShutDown"
)

// 默认的 Martini 对象
var safeMartini = martini.New()

// 临时保存 safeMartini 的 handlers
var cacheHandlers = []martini.Handler{}

var started bool

func appStart() bool {
	return started
}

// +dl en
// Handler add handler to builtin *Martini
// +dl

// 给内建 *Martini 对象添加 handler
func Handler(handler ...martini.Handler) {
	if !appStart() {
		cacheHandlers = append(cacheHandlers, handler...)
	}
}

// +dl en
// Martini returns builtin *Martini and master Router.
// call once, returns nil to again.
// +dl

/*
  返回内建 *Martini 和主 Router, 只能调用一次, 再次调用返回 nil.
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
	cacheHandlers = nil
	callInit()
	return safeMartini, safeRouter
}

var notifyMaps map[string][]int
var notifyFn []func(os.Signal) bool

/*
  ListenSignal 为监听 sigs 信号增加执行函数.
  参数:
 	fn 执行函数, 返回值如果是 true, 表示触发后剔除此函数.
 	sigs 为一组要监听的信号, 支持系统信号和自定义信号.
*/
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

var signalLock sync.RWMutex

/*
  FireSignal 按照 LIFO 的次序调用 Listen 增加的监听函数.
  如果捕获到 panic 中断调用, 并且监听函数会被剔除.
  参数:
	sig 指示触发信号
	remove 指示触发后是否剔除掉所有的触发函数
*/
func FireSignal(sig os.Signal, remove bool) {

	signalLock.RLock()
	idx := notifyMaps[sig.String()]
	signalLock.RUnlock()

	if remove {
		signalLock.Lock()
		delete(notifyMaps, sig.String())
		signalLock.Unlock()
	}

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
		if remove || clear || err != nil {
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
		FireSignal(<-ch, false)
	}
}

// Recover 执行函数 fn, 返回 recover() 结果
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
	RouterGet = martini.NewRouter()
	RouterPut = martini.NewRouter()
	RouterHead = martini.NewRouter()
	RouterPost = martini.NewRouter()
	RouterAjax = martini.NewRouter()
	RouterPatch = martini.NewRouter()
	RouterDelete = martini.NewRouter()
	RouterOptions = martini.NewRouter()
	RouterAny = martini.NewRouter()
	safeRouter.NotFound(RouterDispatch, RouterAny.Handle)
}

// +dl en
// RouterDispatch for master Router, auto dispatch RouterXxxx router.
// +dl

// RouterDispatch 仅用于主路由, 根据 req.Method 分派子路由.
func RouterDispatch(res http.ResponseWriter, req *http.Request, c martini.Context) {
	switch req.Method {
	case "GET":
		RouterGet.Handle(res, req, c)
	case "PUT":
		RouterPut.Handle(res, req, c)
	case "HEAD":
		RouterHead.Handle(res, req, c)
	case "POST":
		if req.Header.Get("X-Requested-With") == "XMLHttpRequest" {
			RouterAjax.Handle(res, req, c)
		} else {
			RouterPost.Handle(res, req, c)
		}

	case "PATCH":
		RouterPatch.Handle(res, req, c)
	case "DELETE":
		RouterDelete.Handle(res, req, c)
	case "OPTIONS":
		RouterOptions.Handle(res, req, c)
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

var rolesAll = []string{}

/*
  RolesSet 设置字符串角色名称集合, 用于角色控制.
  如果要启用角色控制, 必须在注册路由之前进行设置.
  字符串值会被转化为小写, 排序, 剔重.
  为 accessflags 传递 types.Role 值做准备.
*/
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

// 依据 RolesSet 设置的字符串角色集合对参数 rs 进行计算, 返回 types.Role 值.
func RolesToRole(rs []string) (x Role) {
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

// +dl en
// role-based access control
// +dl

/*
  RBAC 返回用于角色控制的 Handler
  依据 RolesSet 设置的字符串角色集合对参数 rs 进行计算,
  得到 types.Role 值并使用 accessflags 生成 Handler
*/
func RBAC(rs []string) martini.Handler {
	return accessflags.Forbidden(RolesToRole(rs))
}
