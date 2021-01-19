package request

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	vutil "github.com/suconghou/videoproxy/util"
)

var (
	client = vutil.MakeClient("VIDEO_PROXY", time.Second*15)

	headers = http.Header{
		"User-Agent":      []string{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/86.0.4240.75 Safari/537.36"},
		"Accept-Language": []string{"zh-CN,zh;q=0.9,en;q=0.8"},
	}

	bufferPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 32*1024))
		},
	}
	bytePool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 32*1024)
		},
	}
)

// LockGeter for http cache & lock get
type LockGeter struct {
	time   time.Time
	cache  time.Duration
	caches sync.Map
}

type cacheItem struct {
	time   time.Time
	ctx    context.Context
	cancel context.CancelFunc
	data   *bytes.Buffer
	err    error
}

// NewLockGeter create new lockgeter
func NewLockGeter(cache time.Duration) *LockGeter {
	return &LockGeter{
		time:   time.Now(),
		cache:  cache,
		caches: sync.Map{},
	}
}

// Get with lock & cache,the return bytes is readonly
func (l *LockGeter) Get(url string) ([]byte, error) {
	var now = time.Now()
	l.clean()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t, loaded := l.caches.LoadOrStore(url, &cacheItem{
		time:   now,
		ctx:    ctx,
		cancel: cancel,
		err:    fmt.Errorf("timeout"),
	})
	v := t.(*cacheItem)
	if loaded {
		<-v.ctx.Done()
		return v.data.Bytes(), v.err
	}
	data, err := Get(url)
	v.data = data
	v.err = err
	cancel()
	return data.Bytes(), err
}

func (l *LockGeter) clean() {
	var now = time.Now()
	if now.Sub(l.time) < time.Minute {
		return
	}
	l.caches.Range(func(key, value interface{}) bool {
		var v = value.(*cacheItem)
		if now.Sub(v.time) > l.cache {
			v.cancel()
			bufferPool.Put(v.data)
			l.caches.Delete(key)
		}
		return true
	})
	l.time = now
}

// Get http data, the return value should be readonly
func Get(url string) (*bytes.Buffer, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header = headers
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s:%s", url, resp.Status)
	}
	var (
		buffer = bufferPool.Get().(*bytes.Buffer)
		buf    = bytePool.Get().([]byte)
	)
	buffer.Reset()
	_, err = io.CopyBuffer(buffer, resp.Body, buf)
	bytePool.Put(buf)
	return buffer, err
}
