package request

import (
	"strconv"
	"strings"
	"time"

	"github.com/suconghou/mediaindex"
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
	var indexEndOffset uint64
	var totalSize uint64
	indexEndOffset, err = strconv.ParseUint(item.IndexRange.End, 10, 64)
	if err != nil {
		return err
	}
	totalSize, err = strconv.ParseUint(item.ContentLength, 10, 64)
	if err != nil {
		return err
	}
	return parseWebm(bs, indexEndOffset, totalSize)
}

func parseMp4(bs []byte) error {
	mediaindex.ParseMp4(bs)
}

func parseWebm(bs []byte, indexEndOffset uint64, totalSize uint64) error {
	mediaindex.ParseWebm(bs)
}
