package sapi

import (
	"fmt"
	"io"
	"net/http"
	"runtime"
	"runtime/debug"

	"wgf/conf"
)

type Sapi struct {
	server *Server

	//Description
	Name     string
	FullName string

	//Config
	BaseConfig    *conf.Conf
	RuntimeConfig conf.Conf

	//by golang http package
	Res http.ResponseWriter
	Req *http.Request

	//IO
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	//Response Header Status
	Status int

	//Plugins
	plugins map[string]interface{}

	actionName string

	//about runtime
	requestChannel chan int
}

func (p *Sapi) SetActionName(name string) {
	p.actionName = name
}

//中止当前请求，之后的代码不会再执行。但plugin的requestShutdown会执行。
//建议只在action逻辑中执行。
func (p *Sapi) ExitRequest() {
	p.requestChannel <- 1
	runtime.Goexit()
}

//输出内容给客户端，第一次输出之前会先输出header信息
func (p *Sapi) Print(val interface{}) {
	fmt.Fprint(p.Stdout, val)
}

func (p *Sapi) Println(val interface{}) {
	fmt.Fprintln(p.Stdout, val)
}

//获取与当前请求相关的plugin，通常是指针。
func (p *Sapi) Plugin(name string) interface{} {
	return p.plugins[name]
}

func (p *Sapi) Log(log interface{}) {
	p.server.Log(log)
}

func (p *Sapi) RequestURI() string {
	return p.Req.RequestURI
}

func (p *Sapi) start(c chan int) {
	p.requestChannel = c
	defer p.ExitRequest()

	defer func() {
		r := recover()
		if nil != r {
			p.Log(r)
			debug.PrintStack()
		}
	}()

	pluginOrders := GetPluginOrder()
	for _, name := range pluginOrders {
		p.pluginRequestInit(name)
	}

	//execute action
	action, actionErr := GetAction(p.actionName)
	if nil != actionErr {
		p.Log("URI[" + p.Req.URL.String() + "] " + actionErr.Error())
		return
	}

	action.SetSapi(p)
	if !action.UseSpecialMethod() {
		action.Execute()
	} else {
		switch p.Req.Method {
		case "GET":
			action.DoGet()
		case "POST":
			action.DoPost()
		}
	}

	//request shutdown
	for i := len(pluginOrders) - 1; i >= 0; i-- {
		p.pluginRequestShutdown(pluginOrders[i])
	}
}

func (p *Sapi) pluginRequestInit(name string) {
	info, ok := pluginMap[name]
	if ok {
		obj, _ := info.Creater()
		if nil != info.HookPluginRequestInit {
			info.HookPluginRequestInit(p, obj)
		}
		p.plugins[name] = obj
	}
}

func (p *Sapi) pluginRequestShutdown(name string) {
	info, ok := pluginMap[name]
	if ok {
		obj, _ := p.plugins[name]
		if nil != info.HookPluginRequestShutdown {
			info.HookPluginRequestShutdown(p, obj)
		}
		delete(p.plugins, name)
	}
}

func NewSapi(pServer *Server, res http.ResponseWriter, req *http.Request) *Sapi {
	s := &Sapi{}
	s.Name = "fcgi"
	s.FullName = "Wgf FastCgi Server API"
	s.plugins = make(map[string]interface{})

	s.server = pServer
	s.Res = res
	s.Req = req

	s.Stdout = res
	s.Stderr = res
	s.Stdin = req.Body

	return s
}
