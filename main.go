package main

import (
	"net/http"
	_ "net/http/pprof"
	"videortc/rtc"
	"videortc/ws"

	"videortc/util"
)

func main() {

	go func() {
		http.ListenAndServe("0.0.0.0:7786", nil)
	}()

	manager := rtc.NewPeerManager()

	var init = func(msg *ws.InitEvent) error {
		util.Log.Print(msg)
		return nil
	}
	var online = func(msg *ws.OnlineEvent) error {
		_, err := manager.Ensure(msg.ID)
		return err
	}
	var umsg = func(msg *ws.MsgEvent) error {
		return manager.Dispatch(msg)
	}
	signal := &ws.Peer{
		ID:           "zlnj1q6h-2hmf-1fmc-2ajh-20mx2hxk1r6r",
		OnInit:       init,
		OnUserOnline: online,
		OnUserMsg:    umsg,
	}
	manager.SetSignal(signal)
	signal.Loop()
}
