package main

import (
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/chzyer/flagx"
	"gopkg.in/logex.v1"
)

type Config struct {
	Proxy     []string `flag:"p;usage=proxy"`
	Server    string   `flag:"s;usage=godl will enter server mode if specified listen addr with -s"`
	Overwrite bool     `flag:"f;usage=overwritten if file is exists, false mean resume the progress from the meta file"`
	MaxSpeed  int64    `flag:"max;usage=max speed"`
	ConnSize  int      `flag:"n;def=5;usage=specified the max connections connected"`
	BlockBit  uint     `flag:"b;def=20;usage=block size represented by bit"`

	Meta     bool `flag:"usage=print meta"`
	Progress bool `flag:"np;def=true;usage=show progress"`
	Debug    bool `flag:"v;usage=turn on debug mode"`

	Url2    string   `flag:"u;usage=url, same as specified at arg"`
	Url     string   `flag:"[0];usage=url"`
	Headers []string `flag:"H"`

	obj *flagx.Object
}

func NewConfig() *Config {
	var c Config
	c.obj = flagx.Parse(&c)
	if c.Url == "" && c.Url2 != "" {
		c.Url = c.Url2
	}
	logex.ShowCode = c.Debug
	return &c
}

func singleDn(c *Config, cwd string) {
	if c.Url == "" {
		c.obj.Usage()
		return
	}
	tcfg := &TaskConfig{
		Clean:      c.Overwrite,
		MaxSpeed:   c.MaxSpeed,
		Progress:   c.Progress,
		Proxy:      c.Proxy,
		ShowRealSp: c.Debug,
		Headers:    c.Headers,
	}

	task, err := NewDnTaskAuto(c.Url, cwd, c.BlockBit, tcfg)
	if err != nil {
		logex.Fatal(err)
	}
	if c.Meta {
		task.Meta.header = nil
		for i := range task.Meta.Blocks {
			if task.Meta.Blocks[i] == nil {
				task.Meta.Blocks = task.Meta.Blocks[:i]
				break
			}
		}
		logex.Pretty(task.Meta)
		return
	}

	closeSignal := make(chan os.Signal, 1)
	go func() {
		task.Schedule(c.ConnSize)
		closeSignal <- os.Interrupt
	}()
	signal.Notify(closeSignal,
		os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGHUP)
	<-closeSignal
	task.Close()

	if task.Meta.IsFinish() {
		task.Meta.Remove()
	} else {
		task.Meta.Sync()
	}
}

func main() {
	runtime.GOMAXPROCS(4)
	c := NewConfig()
	cwd, err := os.Getwd()
	if err != nil {
		logex.Fatal(err)
	}

	if c.Server != "" {
		mux := http.NewServeMux()
		bindHandler(mux)
		err := http.ListenAndServe(c.Server, mux)
		if err != nil {
			logex.Fatal(err)
		}
		return
	}

	singleDn(c, cwd)
}
