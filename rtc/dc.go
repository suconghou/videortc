package rtc

import (
	"io"
	"time"
	"videortc/util"
	"videortc/video"

	"github.com/pion/webrtc/v3"
)

var (
	dcResolveMsg = make(chan *resolveEvent)
	dcPingMsg    = make(chan *pingPongEvent)
	dcQuitMsg    = make(chan *quitEvent)
	worker       = make(chan func() error)
	vHub         = video.NewMediaHub()
)

type vinfos struct {
	ID    string   `json:"id"`
	Parts []uint64 `json:"parts"`
}

// 收到此响应需要队列回复他二进制
type resolveEvent struct {
	dc *webrtc.DataChannel
	vinfos
}

type quitEvent struct {
	dc *webrtc.DataChannel
	vinfos
}

// ping/pong 共用
type pingPongEvent struct {
	dc    *webrtc.DataChannel
	Event string `json:"event"`
}

func init() {
	go waitMsg()
	go workerLoop()
}

func waitMsg() {
	for {
		select {
		case data := <-dcResolveMsg:
			fn := func() error {
				return vHub.Response(data.dc, data.ID, data.Parts)
			}
			select {
			case worker <- fn:
			case <-time.After(time.Second):
				go runIt(fn)
			}

		case data := <-dcPingMsg:
			fn := func() error {
				return sendPong(data.dc)
			}
			select {
			case worker <- fn:
			case <-time.After(time.Second):
				go runIt(fn)
			}
		case data := <-dcQuitMsg:
			fn := func() error {
				vHub.QuitResponse(data.dc, data.ID, data.Parts)
				return nil
			}
			select {
			case worker <- fn:
			case <-time.After(time.Second):
				go runIt(fn)
			}

		}
	}
}

func workerLoop() {
	for fn := range worker {
		runIt(fn)
	}
}

func runIt(fn func() error) {
	if err := fn(); err != nil {
		util.Log.Print(err)
	}
}

func sendPong(d *webrtc.DataChannel) error {
	if badDc(d.ReadyState()) {
		return nil
	}
	return d.SendText(`{"event":"pong"}`)
}

func sendPing(d *webrtc.DataChannel) error {
	if d == nil {
		return io.ErrClosedPipe
	}
	if badDc(d.ReadyState()) {
		return nil
	}
	return d.SendText(`{"event":"ping"}`)
}
