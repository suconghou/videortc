package request

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/suconghou/mediaindex"

	"github.com/suconghou/youtubevideoparser"
)

var (
	infoMapCache sync.Map
	httpProvider = NewLockGeter(time.Minute)
	baseURL      = os.Getenv("BASE_URL")
)

type infoItem struct {
	time time.Time
	data map[int][2]uint64
}

func cacheGet(key string) map[int][2]uint64 {
	v, ok := infoMapCache.Load(key)
	if ok {
		return v.(*infoItem).data
	}
	return nil
}

func cacheSet(key string, val map[int][2]uint64) {
	var now = time.Now()
	infoMapCache.Range(func(key, value interface{}) bool {
		var item = value.(*infoItem)
		if now.Sub(item.time) > time.Hour {
			infoMapCache.Delete(key)
		}
		return true
	})
	infoMapCache.Store(key, &infoItem{
		time: now,
		data: val,
	})
}

// GetIndex with cache
func GetIndex(vid string, item *youtubevideoparser.StreamItem, index int) (string, error) {
	var key = fmt.Sprintf("%s:%s", vid, item.Itag)
	ranges := cacheGet(key)
	if ranges == nil {
		var err error
		ranges, err = parseIndex(vid, item)
		if err != nil {
			return "", err
		}
		cacheSet(key, ranges)
	}
	info := ranges[index]
	if info[1] == 0 {
		return "", fmt.Errorf("%s:%s error get %d index range", vid, item.Itag, index)
	}
	return getData(vid, item.Itag, int(info[0]), int(info[1]), item), nil
}

// parse item media
func parseIndex(vid string, item *youtubevideoparser.StreamItem) (map[int][2]uint64, error) {
	start, err := strconv.Atoi(item.IndexRange.Start)
	if err != nil {
		return nil, err
	}
	end, err := strconv.Atoi(item.IndexRange.End)
	if err != nil {
		return nil, err
	}
	bs, err := httpProvider.Get(getData(vid, item.Itag, start, end+1, item))
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
func getData(vid string, itag string, start int, end int, item *youtubevideoparser.StreamItem) string {
	if baseURL != "" {
		return getByUpstream(baseURL, vid, itag, start, end)
	}
	return getByOrigin(item, start, end)
}

func getByUpstream(baseURL string, vid string, itag string, start int, end int) string {
	return fmt.Sprintf("%s/%s/%s/%d-%d.ts", baseURL, vid, itag, start, end-1)
}

func getByOrigin(item *youtubevideoparser.StreamItem, start int, end int) string {
	return fmt.Sprintf("%s&range=%d-%d", item.URL, start, end-1)
}

// GetInfoByUpstream 媒体索引也用upstream
func GetInfoByUpstream(baseURL string, vid string) (*youtubevideoparser.VideoInfo, error) {
	var url = fmt.Sprintf("%s/%s.json", baseURL, vid)
	bs, err := httpProvider.Get(url)
	if err != nil {
		return nil, err
	}
	var data *youtubevideoparser.VideoInfo
	err = json.Unmarshal(bs, &data)
	return data, err
}
