package video

import (
	"log"
	"strings"
	"sync"
	"time"
	"videortc/request"

	"github.com/pion/webrtc/v3"

	"github.com/suconghou/videoproxy/util"
	"github.com/suconghou/youtubevideoparser"
)

const chunk = 51200

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

func (m *MediaHub) getVideoInfo(id string) (*videoItem, *youtubevideoparser.StreamItem) {
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
			return nil, nil
		}
		info = &videoItem{
			vinfo,
			now,
		}
		m.lock.Lock()
		m.videos[id] = info
		m.lock.Unlock()
	}
	return info, info.Streams[itag]
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
	// TODO start send chunk bs
	log.Print(bs)
	return nil
}

// QuitResponse cancel that send task
func (m *MediaHub) QuitResponse(id string, index uint64) error {
	return nil
}

func (m *MediaHub) getMediaMap() {

}
