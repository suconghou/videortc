package ws

import (
	"crypto/tls"
	"net/http"
	"time"

	"videortc/util"

	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

// 我上线后，当前swarmIds中的用户返回给我，我等待他们的链接
type InitEvent struct {
	IDS []string
}

// 当有用户上线，我需要链接他
type OnlineEvent struct {
	ID string
}

// Event 可能的值：candidate/offer/answer ，用于webrtc信令传递
type MsgEvent struct {
	From  string
	To    string
	Event string
	Data  gjson.Result
}

// WsPeer 代表一个ws链接
type WsPeer struct {
	ID           string
	OnInit       func(msg *InitEvent)
	OnUserOnline func(msg *OnlineEvent)
	OnUserMsg    func(msg *MsgEvent)
	conn         *websocket.Conn
	initMsg      chan *InitEvent
	onlineMsg    chan *OnlineEvent
	userMsg      chan *MsgEvent
	send         chan map[string]interface{}
}

var (
	// defaultDialer is a dialer with all fields set to the default values.
	defaultDialer = &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	}
)

// Loop msg
func (p *WsPeer) Loop(addr string) {
	p.initMsg = make(chan *InitEvent)
	p.onlineMsg = make(chan *OnlineEvent)
	p.userMsg = make(chan *MsgEvent)
	p.send = make(chan map[string]interface{})
	// 读线程
	go func() {
		for {
			util.Log.Print(p.wsMsgLoop(addr))
			time.Sleep(time.Second)
		}
	}()
	// 写线程
	go func() {
		var (
			timer = time.NewTicker(time.Minute)
			err   error
		)
		for {
			select {
			case data := <-p.send:
				if p.conn == nil {
					continue
				}
				if err = p.conn.SetWriteDeadline(time.Now().Add(time.Second * 3)); err != nil {
					util.Log.Print(err)
				}
				if err = p.conn.WriteJSON(data); err != nil {
					util.Log.Print(err)
				}
				if err = p.conn.SetWriteDeadline(time.Now().Add(time.Hour)); err != nil {
					util.Log.Print(err)
				}
			case <-timer.C:
				if p.conn == nil {
					continue
				}
				if err = p.conn.WriteControl(websocket.PingMessage, []byte(""), time.Now().Add(time.Second)); err != nil {
					util.Log.Print(err)
					p.conn.Close()
				}
			}
		}
	}()
	for {
		select {
		case data := <-p.initMsg:
			p.OnInit(data)
		case data := <-p.onlineMsg:
			p.OnUserOnline(data)
		case data := <-p.userMsg:
			p.OnUserMsg(data)
		}
	}
}

// Send answer by ws connection
func (p *WsPeer) Send(data map[string]interface{}) {
	p.send <- data
}

func (p *WsPeer) wsMsgLoop(addr string) error {
	c, _, err := defaultDialer.Dial(addr+p.ID, nil)
	if err != nil {
		return err
	}
	c.SetPongHandler(func(data string) error {
		return c.SetReadDeadline(time.Now().Add(time.Hour))
	})
	p.conn = c
	var (
		messageType int
		data        []byte
		g           gjson.Result
		ev          string
	)
	for {
		messageType, data, err = c.ReadMessage()
		if err != nil {
			break
		}
		if messageType != websocket.TextMessage {
			continue
		}
		g = gjson.ParseBytes(data)
		ev = g.Get("event").String()
		if ev == "online" {
			id := g.Get("id").String()
			if id != "" {
				p.onlineMsg <- &OnlineEvent{id}
			}
		} else if ev == "init" {
			var ids = []string{}
			g.Get("ids").ForEach(func(key gjson.Result, value gjson.Result) bool {
				if id := value.String(); id != "" {
					ids = append(ids, id)
				}
				return true
			})
			p.initMsg <- &InitEvent{ids}
		} else if ev != "" {
			from := g.Get("from").String()
			to := g.Get("to").String()
			if from != "" && to != "" {
				p.userMsg <- &MsgEvent{
					From:  from,
					To:    to,
					Event: ev,
					Data:  g.Get("data"),
				}
			}
		} else {
			util.Log.Print(string(data))
		}
	}
	c.Close()
	return err
}
