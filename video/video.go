package video

import (
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"

	"github.com/suconghou/videoproxy/util"
	"github.com/suconghou/youtubevideoparser"
)

var (
	videoClient = util.MakeClient("VIDEO_PROXY", time.Minute)
)

type videoItem struct {
	*youtubevideoparser.VideoInfo
	time time.Time
}

// MediaHub manage all videos
type MediaHub struct {
	lock   *sync.RWMutex
	videos map[string]*videoItem
}

// NewMediaHub create MediaHub
func NewMediaHub() *MediaHub {
	return &MediaHub{
		lock:   &sync.RWMutex{},
		videos: map[string]*videoItem{},
	}
}

// Ok test if this resource ok, TODO with cache
func (m *MediaHub) Ok(id string) bool {
	var arr = strings.Split(id, ":")
	if len(arr) != 2 {
		return false
	}
	var vid = arr[0]
	var itag = arr[1]
	info, err := m.get(vid)
	if err != nil {
		return false
	}
	if item, ok := info.Streams[itag]; ok {
		if item.InitRange == nil || item.IndexRange == nil || item.ContentLength == "" {
			return false
		}
		m.lock.Lock()
		m.videos[vid] = &videoItem{
			info,
			time.Now(),
		}
		m.lock.Unlock()
		return true
	}
	return false
}

func (m *MediaHub) get(id string) (*youtubevideoparser.VideoInfo, error) {
	parser, err := youtubevideoparser.NewParser(id, videoClient)
	if err != nil {
		return nil, err
	}
	return parser.Parse()
}

// Response create send task that send data to dc
func (m *MediaHub) Response(d *webrtc.DataChannel, id string, index uint64) error {
	return nil
}

// QuitResponse cancel that send task
func (m *MediaHub) QuitResponse(id string, index uint64) error {
	return nil
}
