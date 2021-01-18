package ws

import (
	"time"

	"videortc/util"

	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

// InitEvent mean myself online , give me who is online
type InitEvent struct {
	IDS []string
}

// OnlineEvent mean someone online
type OnlineEvent struct {
	ID string
}

// MsgEvent mean candidate/offer/answer types messages
type MsgEvent struct {
	From  string
	To    string
	Event string
	Data  gjson.Result
}

// Peer mean one ws conn
type Peer struct {
	ID           string
	OnInit       func(msg *InitEvent)
	OnUserOnline func(msg *OnlineEvent)
	OnUserMsg    func(msg *MsgEvent)
	conn         *websocket.Conn
	initMsg      chan *InitEvent
	onlineMsg    chan *OnlineEvent
	userMsg      chan *MsgEvent
	send         chan interface{}
}

// Loop msg
func (p *Peer) Loop(addr string) {
	p.initMsg = make(chan *InitEvent)
	p.onlineMsg = make(chan *OnlineEvent)
	p.userMsg = make(chan *MsgEvent)
	p.send = make(chan interface{})
	var wsMsgWorker = make(chan func())
	go func() {
		for fn := range wsMsgWorker {
			fn()
		}
	}()
	go func() {
		var err error
		for {
			select {
			case data := <-p.send:
				err = p.conn.SetWriteDeadline(time.Now().Add(time.Second * 3))
				if err != nil {
					util.Log.Print(err)
				}
				err = p.conn.WriteJSON(data)
				if err != nil {
					util.Log.Print(err)
				}
				err = p.conn.SetWriteDeadline(time.Now().Add(time.Hour))
				if err != nil {
					util.Log.Print(err)
				}
			case <-time.After(time.Minute):
				if err = p.conn.WriteControl(websocket.PingMessage, []byte(""), time.Now().Add(time.Second)); err != nil {
					util.Log.Print(err)
				}
			}
		}
	}()
	go p.connLoop(addr)
	for {
		select {
		case data := <-p.initMsg:
			wsMsgWorker <- func() {
				p.OnInit(data)
			}
		case data := <-p.onlineMsg:
			wsMsgWorker <- func() {
				p.OnUserOnline(data)
			}
		case data := <-p.userMsg:
			wsMsgWorker <- func() {
				p.OnUserMsg(data)
			}
		}
	}
}

// Send answer by ws connection
func (p *Peer) Send(data interface{}) {
	p.send <- data
}

func (p *Peer) connLoop(addr string) {
	for {
		util.Log.Print(p.wsMsgLoop(addr))
		time.Sleep(time.Second)
	}
}

func (p *Peer) wsMsgLoop(addr string) error {
	c, _, err := websocket.DefaultDialer.Dial(addr+p.ID, nil)
	if err != nil {
		return err
	}
	p.conn = c
	var (
		messageType int
		data        []byte
		g           gjson.Result
		ev          string
	)
	for {
		c.SetReadDeadline(time.Now().Add(time.Hour))
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
