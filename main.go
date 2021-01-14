package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"time"
	"videortc/rtc"
	"videortc/ws"

	"videortc/route"
	"videortc/util"
)

var (
	startTime = time.Now()
	manager   *rtc.PeerManager
)

var sysStatus struct {
	Uptime       string
	GoVersion    string
	Hostname     string
	MemAllocated uint64 // bytes allocated and still in use
	MemTotal     uint64 // bytes allocated (even if freed)
	MemSys       uint64 // bytes obtained from system
	NumGoroutine int
	CPUNum       int
	Pid          int
}

func main() {
	var (
		port = flag.Int("p", 6060, "listen port")
		host = flag.String("h", "", "bind address")
	)
	flag.Parse()
	util.Log.Fatal(serve(*host, *port))
}

func serve(host string, port int) error {
	http.HandleFunc("/", routeMatch)
	http.HandleFunc("/status", status)
	http.HandleFunc("/peers", peers)
	util.Log.Printf("Starting up on port %d", port)
	go webrtcLoop()
	return http.ListenAndServe(fmt.Sprintf("%s:%d", host, port), nil)
}

func routeMatch(w http.ResponseWriter, r *http.Request) {
	for _, p := range route.Route {
		if p.Reg.MatchString(r.URL.Path) {
			if err := p.Handler(w, r, p.Reg.FindStringSubmatch(r.URL.Path)); err != nil {
				util.Log.Print(r.URL.Path, " ", err)
			}
			return
		}
	}
	fallback(w, r)
}

func fallback(w http.ResponseWriter, r *http.Request) {
	const index = "index.html"
	files := []string{index}
	if r.URL.Path != "/" {
		files = []string{r.URL.Path, path.Join(r.URL.Path, index)}
	}
	if !tryFiles(files, w, r) {
		if !tryFiles([]string{index}, w, r) {
			http.NotFound(w, r)
		}
	}
}

func tryFiles(files []string, w http.ResponseWriter, r *http.Request) bool {
	for _, file := range files {
		realpath := filepath.Join("./public", file)
		if f, err := os.Stat(realpath); err == nil {
			if f.Mode().IsRegular() {
				http.ServeFile(w, r, realpath)
				return true
			}
		}
	}
	return false
}

func status(w http.ResponseWriter, r *http.Request) {
	memStat := new(runtime.MemStats)
	runtime.ReadMemStats(memStat)
	sysStatus.Uptime = time.Since(startTime).String()
	sysStatus.NumGoroutine = runtime.NumGoroutine()
	sysStatus.MemAllocated = memStat.Alloc / 1024  // 当前内存使用量
	sysStatus.MemTotal = memStat.TotalAlloc / 1024 // 所有被分配的内存
	sysStatus.MemSys = memStat.Sys / 1024          // 内存占用量
	sysStatus.CPUNum = runtime.NumCPU()
	sysStatus.GoVersion = runtime.Version()
	sysStatus.Hostname, _ = os.Hostname()
	sysStatus.Pid = os.Getpid()
	util.JSONPut(w, sysStatus)
}

func peers(w http.ResponseWriter, r *http.Request) {
	var stat = manager.Stats()
	util.JSONPut(w, stat)
}

func webrtcLoop() {
	manager = rtc.NewPeerManager()
	var init = func(msg *ws.InitEvent) {
		// 我上线后别人会主动链接我,我只需要预先为这些peer创建资源,等待MsgEvent发来的offer
		for _, id := range msg.IDS {
			peer, created, err := manager.Ensure(id)
			if err != nil {
				util.Log.Print(err)
				return
			}
			if !created {
				if err := peer.Ping(); err != nil {
					util.Log.Print(err)
				}
			}
		}
	}
	var online = func(msg *ws.OnlineEvent) {
		// 对方刷新页面上线,或者ws重连上线,如果是ws重连上线,这个连接还没断开,则不需要做其他操作
		peer, created, err := manager.Ensure(msg.ID)
		if err != nil {
			util.Log.Print(err)
			return
		}
		if created {
			err = peer.Connect(msg.ID)
			if err != nil {
				util.Log.Print(err)
			}
			return
		}
		err = peer.Ping()
		if err != nil {
			util.Log.Print(err)
		}
	}
	var umsg = func(msg *ws.MsgEvent) {
		if err := manager.Dispatch(msg); err != nil {
			util.Log.Print(err)
		}
	}
	signal := &ws.Peer{
		ID:           "zznj1q6h-2hmf-1fmc-2ajh-20mx2hxk1r6r",
		OnInit:       init,
		OnUserOnline: online,
		OnUserMsg:    umsg,
	}
	manager.SetSignal(signal)
	signal.Loop()
}
