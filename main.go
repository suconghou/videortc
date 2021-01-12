package main

import (
	"net/http"
	_ "net/http/pprof"
	"videortc/rtc"
	"videortc/ws"
)

func main() {

	go func() {
		http.ListenAndServe("0.0.0.0:7786", nil)
	}()

	manager := rtc.NewPeerManager()

	var init = func(msg *ws.InitEvent) error {
		// 我上线后别人会主动链接我,我只需要预先为这些peer创建资源,等待MsgEvent发来的offer
		for _, id := range msg.IDS {
			_, err := manager.Ensure(id)
			if err != nil {
				return err
			}
		}
		return nil
	}
	var online = func(msg *ws.OnlineEvent) error {
		// 别人上线后,需要我主动链接他
		peer, err := manager.Ensure(msg.ID)
		if err != nil {
			return err
		}
		return peer.Connect(msg.ID)
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
