package rtc

import (
	"fmt"
	"sync"
	"time"

	"videortc/util"
	"videortc/ws"

	"github.com/pion/webrtc/v3"
)

var (
	config = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:119.29.1.39:3478"},
			},
		},
	}
)

// Peer mean rtc peer
type Peer struct {
	ws   *ws.Peer
	conn *webrtc.PeerConnection
}

// PeerManager manage every user peer
type PeerManager struct {
	ws    *ws.Peer
	peers map[string]*Peer
	lock  *sync.RWMutex
}

// NewPeerManager do peer manage
func NewPeerManager() *PeerManager {
	return &PeerManager{
		peers: map[string]*Peer{},
		lock:  &sync.RWMutex{},
	}
}

// SetSignal 设置信令服务器
func (m *PeerManager) SetSignal(ws *ws.Peer) {
	m.ws = ws
}

// Ensure this ws peer connect me
func (m *PeerManager) Ensure(id string) (*Peer, error) {
	var (
		peer *Peer
		ok   bool
		err  error
	)
	m.lock.Lock()
	defer m.lock.Unlock()
	peer, ok = m.peers[id]
	if !ok {
		peer, err = NewPeer(m.ws)
		if err != nil {
			return nil, err
		}
		m.peers[id] = peer
	}
	return peer, nil
}

// Dispatch message to peer
func (m *PeerManager) Dispatch(msg *ws.MsgEvent) error {
	if msg.Event == "offer" {
		// someone send me offer , we should accept that
		peer, err := m.Ensure(msg.From)
		if err != nil {
			return err
		}
		var sdp = msg.Data.Get("sdp").String()
		return peer.Accept(webrtc.SDPTypeOffer, sdp, msg)
	} else if msg.Event == "candidate" {
		peer, err := m.Ensure(msg.From)
		if err != nil {
			return err
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
	} else {
		util.Log.Print(msg)
	}
	return nil
}

// NewPeer create Peer
func NewPeer(sharedWs *ws.Peer) (*Peer, error) {
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}
	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		util.Log.Printf("ICE Connection State has changed: %s\n", connectionState.String())
	})
	// Register data channel creation handling
	peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
		fmt.Printf("New DataChannel %s %d\n", d.Label(), d.ID())

		// Register channel opening handling
		d.OnOpen(func() {
			fmt.Printf("Data channel '%s'-'%d' open. Random messages will now be sent to any connected DataChannels every 5 seconds\n", d.Label(), d.ID())

			for range time.NewTicker(5 * time.Second).C {
				message := `{"event":"test","ids":[]}`
				fmt.Printf("Sending '%s'\n", message)
				// Send the message as text
				sendErr := d.SendText(message)
				if sendErr != nil {
					panic(sendErr)
				}
			}
		})

		// Register text message handling
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			fmt.Printf("Message from DataChannel '%s': '%s'\n", d.Label(), string(msg.Data))
		})
	})

	return &Peer{
		sharedWs,
		peerConnection,
	}, nil

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

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(p.conn)

	// Sets the LocalDescription, and starts our UDP listeners
	err = p.conn.SetLocalDescription(answer)
	if err != nil {
		return err
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete

	// Output the answer in base64 so we can paste it in browser
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
