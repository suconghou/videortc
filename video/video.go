package video

import (
	"context"
	"os"
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
	vinfo  *youtubevideoparser.VideoInfo
	time   time.Time
	ctx    context.Context
	cancel context.CancelFunc
}

// MediaHub manage all videos
type MediaHub struct {
	videos sync.Map
	time   time.Time
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
		videos: sync.Map{},
		time:   time.Now(),
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

// Ok test if this resource ok
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
		vid   = arr[0]
		itag  = arr[1]
		info  *videoItem
		now   = time.Now()
		vinfo *youtubevideoparser.VideoInfo
		err   error
	)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t, loaded := m.videos.LoadOrStore(vid, &videoItem{
		time:   now,
		ctx:    ctx,
		cancel: cancel,
	})
	info = t.(*videoItem)
	if loaded {
		// 说明已存在此任务,我们只需要监听此任务是否已完成(或早已经完成),完成的任务我们获取其属性就好了
		<-info.ctx.Done()
		if info.vinfo == nil || info.vinfo.Streams == nil {
			return nil, nil
		}
		return info.vinfo, info.vinfo.Streams[itag]
	}
	// 否则此任务没有并发,我们第一个执行,需要正常执行然后设置其属性,并标记已执行完成
	vinfo, err = getInfo(vid)
	if err != nil {
		util.Log.Print(err)
	}
	info.vinfo = vinfo
	cancel()
	if vinfo == nil || vinfo.Streams == nil {
		return nil, nil
	}
	return vinfo, vinfo.Streams[itag]
}

func (m *MediaHub) clean() {
	var now = time.Now()
	if now.Sub(m.time) < time.Minute {
		return
	}
	m.videos.Range(func(key, value interface{}) bool {
		var v = value.(*videoItem)
		if v == nil || now.Sub(v.time) > time.Hour {
			m.videos.Delete(key)
		}
		return true
	})
	m.time = now
}

func getInfo(id string) (*youtubevideoparser.VideoInfo, error) {
	var baseURL = os.Getenv("BASE_URL")
	if baseURL != "" {
		return request.GetInfoByUpstream(baseURL, id)
	}
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
	m.clean()
	return nil
}

// Stats output status
func (m *MediaHub) Stats() map[string]*youtubevideoparser.VideoInfo {
	var res = map[string]*youtubevideoparser.VideoInfo{}
	m.videos.Range(func(key, value interface{}) bool {
		v := value.(*videoItem)
		res[key.(string)] = v.vinfo
		return true
	})
	return res
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	return &bufferTask{
		id,
		index,
		buffers,
		ctx,
		cancel,
	}
}
