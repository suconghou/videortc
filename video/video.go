package video

import (
	"context"
	"strings"
	"sync"
	"time"
	"videortc/request"
	"videortc/util"

	"github.com/pion/webrtc/v3"

	vutil "github.com/suconghou/videoproxy/util"
	"github.com/suconghou/youtubevideoparser"
)

const chunk = 51200

var (
	videoClient = vutil.MakeClient("VIDEO_PROXY", time.Second*5)
)

type videoItem struct {
	vinfo *youtubevideoparser.VideoInfo
	time  time.Time
}

// MediaHub manage all videos
type MediaHub struct {
	lock   *sync.RWMutex
	videos map[string]*videoItem
}

type bufferTask struct {
	id      string
	index   uint64
	buffers [][]byte
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewMediaHub create MediaHub
func NewMediaHub() *MediaHub {
	return &MediaHub{
		lock:   &sync.RWMutex{},
		videos: map[string]*videoItem{},
	}
}

func itemValid(item *youtubevideoparser.StreamItem) bool {
	if item == nil {
		return false
	}
	if item.InitRange == nil || item.IndexRange == nil || item.ContentLength == "" || item.IndexRange.End == "" {
		return false
	}
	return true
}

// Ok test if this resource ok, TODO with cache
func (m *MediaHub) Ok(id string) bool {
	item := m.getItemInfo(id)
	return itemValid(item)
}

func (m *MediaHub) getItemInfo(id string) *youtubevideoparser.StreamItem {
	_, item := m.getVideoInfo(id)
	return item
}

func (m *MediaHub) getVideoInfo(id string) (*youtubevideoparser.VideoInfo, *youtubevideoparser.StreamItem) {
	var arr = strings.Split(id, ":")
	if len(arr) != 2 {
		return nil, nil
	}
	var (
		vid  = arr[0]
		itag = arr[1]
		info *videoItem
		now  = time.Now()
	)
	m.lock.RLock()
	info = m.videos[id]
	m.lock.RUnlock()
	if info == nil || now.Sub(info.time) > time.Hour {
		vinfo, err := getInfo(vid)
		if err != nil {
			util.Log.Print(err)
		}
		info = &videoItem{
			vinfo,
			now,
		}
		m.lock.Lock()
		m.videos[id] = info
		m.lock.Unlock()
	}
	if info.vinfo == nil || info.vinfo.Streams == nil {
		return info.vinfo, nil
	}
	return info.vinfo, info.vinfo.Streams[itag]
}

func getInfo(id string) (*youtubevideoparser.VideoInfo, error) {
	parser, err := youtubevideoparser.NewParser(id, videoClient)
	if err != nil {
		return nil, err
	}
	return parser.Parse()
}

// Response create send task that send data to dc
func (m *MediaHub) Response(d *webrtc.DataChannel, id string, index uint64) error {
	vinfo, item := m.getVideoInfo(id)
	if !itemValid(item) {
		return nil
	}
	bs, err := request.GetIndex(vinfo.ID, item, int(index))
	if err != nil {
		return err
	}
	queueManager.send(d, splitBuffer(bs, id, index))
	return nil
}

// QuitResponse cancel that send task
func (m *MediaHub) QuitResponse(d *webrtc.DataChannel, id string, index uint64) error {
	queueManager.quit(d.ID(), id, index)
	return nil
}

func splitBuffer(bs []byte, id string, index uint64) *bufferTask {
	var (
		buffers = [][]byte{}
		start   = 0
		end     = 0
		l       = len(bs)
		data    []byte
	)
	for {
		if start >= l {
			break
		}
		end = start + chunk
		if end > l {
			end = l
		}
		data = bs[start:end]
		buffers = append(buffers, data)
		start = end
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	return &bufferTask{
		id,
		index,
		buffers,
		ctx,
		cancel,
	}
}
