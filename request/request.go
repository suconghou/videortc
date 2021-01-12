package request

import (
	"strings"
	"time"

	"github.com/suconghou/videoproxy/util"

	"github.com/suconghou/youtubevideoparser"
	"github.com/suconghou/youtubevideoparser/request"
)

var (
	client = util.MakeClient("VIDEO_PROXY", time.Minute)
)

// Parse item media
func Parse(item *youtubevideoparser.StreamItem) error {
	var url = item.URL + "&range=" + item.IndexRange.Start + "-" + item.IndexRange.End
	bs, err := request.GetURLData(url, true, client)
	if err != nil {
		return err
	}
	if strings.Contains(item.Type, "mp4") {
		return parseMp4(bs)
	}
	return parseWebm(bs)
}

func parseMp4(bs []byte) error {
	return nil
}

func parseWebm([]byte) error {
	return nil
}
