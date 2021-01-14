package rtc

import (
	"encoding/json"
	"time"
	"videortc/util"
	"videortc/video"

	"github.com/pion/webrtc/v3"
)

var (
	dcQueryMsg   = make(chan *queryEvent)
	dcResolveMsg = make(chan *resolveEvent)
	dcPingMsg    = make(chan *pingPongEvent)
	dcQuitMsg    = make(chan *quitEvent)
	worker       = make(chan func() error)
	vHub         = video.NewMediaHub()
)

type vinfo struct {
	ID    string `json:"id"`
	Index uint64 `json:"index"`
}

// 对方发来此类型
type queryEvent struct {
	dc *webrtc.DataChannel
	vinfo
}

// 收到此响应需要队列回复他二进制
type resolveEvent queryEvent

type quitEvent queryEvent

// 给对方回复found
type foundEvent struct {
	Event string `json:"event"`
	Data  vinfo  `json:"data"`
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
		case data := <-dcQueryMsg:
			fn := func() error {
				if !vHub.Ok(data.ID) {
					return nil
				}
				var v = &foundEvent{
					Event: "found",
					Data: vinfo{
						Index: data.Index,
						ID:    data.ID,
					},
				}
				return sendFound(data.dc, v)
			}
			select {
			case worker <- fn:
			case <-time.After(time.Second):
				go runIt(fn)
			}

		case data := <-dcResolveMsg:
			fn := func() error {
				return vHub.Response(data.dc, data.ID, data.Index)
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
				return vHub.QuitResponse(data.dc, data.ID, data.Index)
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
	return d.SendText(`{"event":"pong"}`)
}

func sendPing(d *webrtc.DataChannel) error {
	return d.SendText(`{"event":"ping"}`)
}

func sendFound(d *webrtc.DataChannel, v *foundEvent) error {
	bs, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return d.SendText(string(bs))
}
