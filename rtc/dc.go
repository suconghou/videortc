package rtc

import (
	"encoding/json"
	"videortc/util"
	"videortc/video"

	"github.com/pion/webrtc/v3"
)

var (
	dcQueryMsg   = make(chan *queryEvent)
	dcResolveMsg = make(chan *resolveEvent)
	dcPingMsg    = make(chan *pingPongEvent)
	dcQuitMsg    = make(chan *quitEvent)
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
}

func waitMsg() {
	for {
		select {
		case data := <-dcQueryMsg:
			if !vHub.Ok(data.ID) {
				return
			}
			var v = &foundEvent{
				Event: "found",
				Data: vinfo{
					Index: data.Index,
					ID:    data.ID,
				},
			}
			if err := sendFound(data.dc, v); err != nil {
				util.Log.Print(err)
			}

		case data := <-dcResolveMsg:
			util.Log.Print(data)
			if err := vHub.Response(data.dc, data.ID, data.Index); err != nil {
				util.Log.Print(err)
			}
		case data := <-dcPingMsg:
			if err := sendPong(data.dc); err != nil {
				util.Log.Print(err)
			}
		case data := <-dcQuitMsg:
			util.Log.Print(data)
			if err := vHub.QuitResponse(data.ID, data.Index); err != nil {
				util.Log.Print(err)
			}
		}
	}
}

func sendPong(d *webrtc.DataChannel) error {
	return d.SendText(`{"event":"pong"}`)
}

func sendFound(d *webrtc.DataChannel, v *foundEvent) error {
	bs, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return d.SendText(string(bs))
}
