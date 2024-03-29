package rtc

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"videortc/util"
	"videortc/video"
	"videortc/ws"

	"github.com/pion/webrtc/v3"
	"github.com/tidwall/gjson"
)

var (
	config = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:119.29.1.39:3478"},
			},
			{
				URLs:       []string{"turn:119.29.1.39:3478"},
				Username:   "su",
				Credential: "su",
			},
		},
	}
	maxPacketLifeTime = uint16(2000)
)

// Peer mean rtc peer
type Peer struct {
	time time.Time
	ws   *ws.Peer
	conn *webrtc.PeerConnection
	dc   *webrtc.DataChannel
}

// PeerManager manage every user peer
type PeerManager struct {
	ws    *ws.Peer
	api   *webrtc.API
	peers map[string]*Peer
	lock  *sync.RWMutex
}

// DataChannelStatus for datachannel
type DataChannelStatus struct {
	ID       string
	Label    string
	State    string
	Buffered uint64
}

// ConnState for conn status
type ConnState struct {
	Time               time.Time
	ConnectionState    string
	ICEConnectionState string
	ICEGatheringState  string
	DataChannelStatus  *DataChannelStatus
	PeerStatus         webrtc.StatsReport
}

// PeerManagerStats for stats
type PeerManagerStats struct {
	ID    string
	Peers map[string]*ConnState
}

func getApi() *webrtc.API {
	var (
		publicIp      = os.Getenv("PUBLICIP")
		arr           = strings.Split(publicIp, ":")
		settingEngine = webrtc.SettingEngine{}
	)
	if publicIp == "" {
		return webrtc.NewAPI()
	}
	if len(arr) == 2 {
		var (
			ip    = arr[0]
			sport = arr[1]
		)
		port, err := strconv.Atoi(sport)
		if err != nil {
			panic(err)
		}
		udpListener, err := net.ListenUDP("udp", &net.UDPAddr{
			IP:   net.IP{0, 0, 0, 0},
			Port: port,
		})
		if err != nil {
			panic(err)
		}
		settingEngine.SetNAT1To1IPs([]string{ip}, webrtc.ICECandidateTypeHost)
		settingEngine.SetICEUDPMux(webrtc.NewICEUDPMux(nil, udpListener))
		return webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))
	}
	settingEngine.SetNAT1To1IPs([]string{publicIp}, webrtc.ICECandidateTypeHost)
	return webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))
}

// NewPeerManager do peer manage
func NewPeerManager() *PeerManager {
	return &PeerManager{
		api:   getApi(),
		peers: map[string]*Peer{},
		lock:  &sync.RWMutex{},
	}
}

// SetSignal 设置信令服务器
func (m *PeerManager) SetSignal(ws *ws.Peer) {
	m.ws = ws
}

// Ensure 确保已存在此Peer的实例
func (m *PeerManager) Ensure(id string) (*Peer, bool, error) {
	var (
		peer *Peer
		ok   bool
		err  error
	)
	m.cleanPeers()
	m.lock.RLock()
	peer, ok = m.peers[id]
	m.lock.RUnlock()
	if ok {
		if isPeerOk(peer) {
			return peer, false, nil
		}
		peer.Close()
	}
	peer, err = m.newPeer()
	if err != nil {
		return nil, true, err
	}
	m.lock.Lock()
	m.peers[id] = peer
	m.lock.Unlock()
	return peer, true, nil
}

// newPeer create Peer based on the api we created
func (m *PeerManager) newPeer() (*Peer, error) {
	peerConnection, err := m.api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}
	var peer = &Peer{
		time.Now(),
		m.ws,
		peerConnection,
		nil,
	}
	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		util.Log.Printf("ICE Connection State has changed: %s\n", connectionState.String())
	})
	// Register data channel creation handling
	peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
		if peer.dc != nil {
			peer.dc.Close()
		}
		initDc(d)
		peer.dc = d
	})

	return peer, nil
}

//get 创建新的Peer实例,如果有旧的则清理它
func (m *PeerManager) getPeer(id string) *Peer {
	var (
		peer *Peer
	)
	m.lock.RLock()
	peer = m.peers[id]
	m.lock.RUnlock()
	return peer
}

// Dispatch message to peer ,此函数不能阻塞太久, Accept 可能耗时5s
func (m *PeerManager) Dispatch(msg *ws.MsgEvent) error {
	if msg.Event == "offer" {
		// someone send me offer , we should accept that
		peer, _, err := m.Ensure(msg.From)
		if err != nil {
			return err
		}
		var sdp = msg.Data.Get("sdp").String()
		return peer.Accept(webrtc.SDPTypeOffer, sdp, msg)
	} else if msg.Event == "candidate" {
		peer := m.getPeer(msg.From)
		if peer == nil {
			return fmt.Errorf("not found peer %s", msg.From)
		}
		var (
			sdpMid        = msg.Data.Get("sdpMid").String()
			sdpMLineIndex = uint16(msg.Data.Get("sdpMLineIndex").Uint())
		)
		candidate := webrtc.ICECandidateInit{
			Candidate:     msg.Data.Get("candidate").String(),
			SDPMid:        &sdpMid,
			SDPMLineIndex: &sdpMLineIndex,
		}
		return peer.conn.AddICECandidate(candidate)
	} else if msg.Event == "answer" {
		peer := m.getPeer(msg.From)
		if peer == nil {
			return fmt.Errorf("peer not found %s", msg.From)
		}
		var desc = webrtc.SessionDescription{
			Type: webrtc.SDPTypeAnswer,
			SDP:  msg.Data.Get("sdp").String(),
		}
		return peer.conn.SetRemoteDescription(desc)
	} else {
		util.Log.Print(msg)
	}
	return nil
}

// cleanPeers delete closed peers
func (m *PeerManager) cleanPeers() {
	m.lock.Lock()
	for k, p := range m.peers {
		if !isPeerOk(p) {
			p.Close()
			delete(m.peers, k)
		}
	}
	m.lock.Unlock()
}

// Stats get status info
func (m *PeerManager) Stats() *PeerManagerStats {
	var peers = map[string]*ConnState{}
	m.lock.RLock()
	for id, peer := range m.peers {
		var dstatus *DataChannelStatus
		if peer.dc != nil {
			dstatus = &DataChannelStatus{
				ID:       fmt.Sprintf("%d", peer.dc.ID()),
				Label:    peer.dc.Label(),
				State:    peer.dc.ReadyState().String(),
				Buffered: peer.dc.BufferedAmount(),
			}
		}
		peers[id] = &ConnState{
			Time:               peer.time,
			ConnectionState:    peer.conn.ConnectionState().String(),
			ICEConnectionState: peer.conn.ICEConnectionState().String(),
			ICEGatheringState:  peer.conn.ICEGatheringState().String(),
			DataChannelStatus:  dstatus,
			PeerStatus:         peer.conn.GetStats(),
		}
	}
	m.lock.RUnlock()
	return &PeerManagerStats{
		ID:    m.ws.ID,
		Peers: peers,
	}
}

// StatsVideo for video cache info
func (m *PeerManager) StatsVideo() *video.VStatus {
	return vHub.Stats()
}

// Accept for some peer send me offer to connect me
func (p *Peer) Accept(sdpType webrtc.SDPType, sdp string, msg *ws.MsgEvent) error {
	p.conn.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		var data = map[string]interface{}{
			"event": "candidate",
			"from":  p.ws.ID,
			"to":    msg.From,
			"data":  candidate.ToJSON(),
		}
		p.ws.Send(data)
	})

	offer := webrtc.SessionDescription{
		Type: sdpType,
		SDP:  sdp,
	}

	// Set the remote SessionDescription
	err := p.conn.SetRemoteDescription(offer)
	if err != nil {
		return err
	}

	// Create an answer
	answer, err := p.conn.CreateAnswer(nil)
	if err != nil {
		return err
	}

	// Sets the LocalDescription, and starts our UDP listeners
	err = p.conn.SetLocalDescription(answer)
	if err != nil {
		return err
	}

	r := p.conn.LocalDescription()
	var data = map[string]interface{}{
		"event": "answer",
		"from":  p.ws.ID,
		"to":    msg.From,
		"data":  r,
	}
	p.ws.Send(data)
	return nil
}

// Connect 主动链接别人, 必须确保这个Peer 处于 new 状态
func (p *Peer) Connect(id string) error {
	var fn = func() error {
		offer, err := p.conn.CreateOffer(nil)
		if err != nil {
			return err
		}
		var data = map[string]interface{}{
			"event": "offer",
			"from":  p.ws.ID,
			"to":    id,
			"data":  offer,
		}
		p.ws.Send(data)
		p.conn.OnICECandidate(func(candidate *webrtc.ICECandidate) {
			if candidate == nil {
				return
			}
			var data = map[string]interface{}{
				"event": "candidate",
				"from":  p.ws.ID,
				"to":    id,
				"data":  candidate.ToJSON(),
			}
			p.ws.Send(data)
		})
		err = p.conn.SetLocalDescription(offer)
		if err != nil {
			return err
		}
		return nil
	}
	p.conn.OnNegotiationNeeded(func() {
		if err := fn(); err != nil {
			util.Log.Print(err)
		}
	})

	dc, err := p.conn.CreateDataChannel("dc", &webrtc.DataChannelInit{MaxPacketLifeTime: &maxPacketLifeTime})
	if err != nil {
		return err
	}
	if p.dc != nil {
		p.dc.Close()
	}
	initDc(dc)
	p.dc = dc
	return nil
}

// Close 关闭peerConection,和dataChannel,但是不影响共享的ws
func (p *Peer) Close() error {
	var err1 error
	var err2 error
	if p.dc != nil {
		err1 = p.dc.Close()
	}
	err2 = p.conn.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

// Ping send ping to dc
func (p *Peer) Ping() error {
	return sendPing(p.dc)
}

func initDc(d *webrtc.DataChannel) {

	// Register channel opening handling
	d.OnOpen(func() {
		util.Log.Printf("Data channel '%s'-'%d' open. \n", d.Label(), d.ID())
		if err := sendPing(d); err != nil {
			util.Log.Print(err)
		}
	})

	d.OnClose(func() {
		util.Log.Printf("Data channel '%s'-'%d' closed. \n", d.Label(), d.ID())
	})

	d.OnError(func(err error) {
		util.Log.Printf("Data channel '%s'-'%d' error %s. \n", d.Label(), d.ID(), err)
	})

	// Register text message handling
	d.OnMessage(func(msg webrtc.DataChannelMessage) {
		if msg.IsString {
			g := gjson.ParseBytes(msg.Data)
			ev := g.Get("event").String()
			if ev == "query" {
				dcQueryMsg <- &queryEvent{
					vinfo: vinfo{
						Index: g.Get("data.index").Uint(),
						ID:    g.Get("data.id").String(),
					},
					dc: d,
				}
				return
			} else if ev == "resolve" {
				dcResolveMsg <- &resolveEvent{
					vinfo: vinfo{
						Index: g.Get("data.index").Uint(),
						ID:    g.Get("data.id").String(),
					},
					dc: d,
				}
				return
			} else if ev == "ping" {
				dcPingMsg <- &pingPongEvent{
					Event: ev,
					dc:    d,
				}
				return
			} else if ev == "quit" {
				dcQuitMsg <- &quitEvent{
					vinfo: vinfo{
						Index: g.Get("data.index").Uint(),
						ID:    g.Get("data.id").String(),
					},
					dc: d,
				}
				return
			} else if ev == "pong" {
				return
			}
		}
		util.Log.Printf("Message from DataChannel '%s'-'%d': '%s'\n", d.Label(), d.ID(), string(msg.Data))

	})

}

func isPeerOk(peer *Peer) bool {
	var cstatus = peer.conn.ConnectionState()
	if cstatus == webrtc.PeerConnectionStateDisconnected || cstatus == webrtc.PeerConnectionStateClosed || cstatus == webrtc.PeerConnectionStateFailed {
		return false
	}
	var i = peer.conn.ICEConnectionState()
	if i == webrtc.ICEConnectionStateDisconnected || i == webrtc.ICEConnectionStateFailed || i == webrtc.ICEConnectionStateClosed {
		return false
	}
	if peer.dc != nil {
		var dstatus = peer.dc.ReadyState()
		if badDc(dstatus) {
			return false
		}
		var g = peer.conn.ICEGatheringState()
		var connecting = dstatus == webrtc.DataChannelStateConnecting && cstatus == webrtc.PeerConnectionStateNew && i == webrtc.ICEConnectionStateNew && g == webrtc.ICEGatheringStateComplete
		if connecting && time.Since(peer.time) > time.Minute {
			return false
		}
	}
	return true
}

func badDc(dstatus webrtc.DataChannelState) bool {
	return dstatus == webrtc.DataChannelStateClosed || dstatus == webrtc.DataChannelStateClosing
}
