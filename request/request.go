package request

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/suconghou/mediaindex"
	vutil "github.com/suconghou/videoproxy/util"

	"github.com/suconghou/youtubevideoparser"
	"github.com/suconghou/youtubevideoparser/request"
)

const baseURL = "http://share.suconghou.cn/video"

var (
	client          = vutil.MakeClient("VIDEO_PROXY", time.Second*15)
	mediaIndexCache sync.Map
)

func cacheGet(key string) map[int][2]uint64 {
	v, ok := mediaIndexCache.Load(key)
	if ok {
		val, ok := v.(map[int][2]uint64)
		if ok {
			return val
		}
	}
	return nil
}

func cacheSet(key string, val map[int][2]uint64) {
	mediaIndexCache.Store(key, val)
}

// GetIndex with cache
func GetIndex(vid string, item *youtubevideoparser.StreamItem, index int) ([]byte, error) {
	var key = fmt.Sprintf("%s:%s", vid, item.Itag)
	ranges := cacheGet(key)
	if ranges == nil {
		var err error
		ranges, err = Parse(vid, item)
		if err != nil {
			return nil, err
		}
		cacheSet(key, ranges)
	}
	info := ranges[index]
	if info[1] == 0 {
		return nil, fmt.Errorf("%s:%s error get %d index range", vid, item.Itag, index)
	}
	return getData(vid, item.Itag, int(info[0]), int(info[1]), item)
}

// Parse item media
func Parse(vid string, item *youtubevideoparser.StreamItem) (map[int][2]uint64, error) {
	start, err := strconv.Atoi(item.IndexRange.Start)
	if err != nil {
		return nil, err
	}
	end, err := strconv.Atoi(item.IndexRange.End)
	if err != nil {
		return nil, err
	}
	bs, err := getData(vid, item.Itag, start, end, item)
	if err != nil {
		return nil, err
	}
	if strings.Contains(item.Type, "mp4") {
		return mediaindex.ParseMp4(bs), nil
	}
	var indexEndOffset uint64
	var totalSize uint64
	indexEndOffset, err = strconv.ParseUint(item.IndexRange.End, 10, 64)
	if err != nil {
		return nil, err
	}
	totalSize, err = strconv.ParseUint(item.ContentLength, 10, 64)
	if err != nil {
		return nil, err
	}
	return mediaindex.ParseWebM(bs, indexEndOffset, totalSize), nil
}

// getData do http request and got vid itag data
func getData(vid string, itag string, start int, end int, item *youtubevideoparser.StreamItem) ([]byte, error) {
	if item == nil {
		return getByUpstream(vid, itag, start, end)
	}
	return getByOrigin(item, start, end)
}

func getByUpstream(vid string, itag string, start int, end int) ([]byte, error) {
	var url = fmt.Sprintf("%s/%s/%s/%d-%d.ts", baseURL, vid, itag, start, end-1)
	return request.GetURLData(url, true, client)
}

func getByOrigin(item *youtubevideoparser.StreamItem, start int, end int) ([]byte, error) {
	var url = fmt.Sprintf("%s&range=%d-%d", item.URL, start, end-1)
	return request.GetURLData(url, true, client)
}
