package request

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
		New: func() any {
			return bytes.NewBuffer(make([]byte, 32*1024))
		},
	}
	errTimeout = errors.New("timeout")

	HttpProvider = NewLockGeter()
)

// LockGeter for http cache & lock get
type LockGeter struct {
	time   int64
	caches sync.Map
}

type cacheItem struct {
	time    int64
	ctx     context.Context
	cancel  context.CancelFunc
	data    []byte // immutable, safe to return to callers
	err     error
	loading bool
}

// NewLockGeter create new lockgeter
func NewLockGeter() *LockGeter {
	return &LockGeter{
		time:   0,
		caches: sync.Map{},
	}
}

// Get with lock & cache,the return bytes is readonly
func (l *LockGeter) Get(url string, ttl int64) ([]byte, error) {
	var now = time.Now().Unix()
	l.clean(now)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t, loaded := l.caches.LoadOrStore(url, &cacheItem{
		time:    now + ttl,
		ctx:     ctx,
		cancel:  cancel,
		err:     errTimeout,
		loading: true,
	})
	v := t.(*cacheItem)
	if loaded {
		<-v.ctx.Done()
		v.loading = false
		if v.data == nil {
			return nil, v.err
		}
		return v.data, v.err
	}
	data, err := getBytes(url)
	v.data = data
	v.err = err
	v.loading = false
	cancel()
	if data == nil {
		return nil, err
	}
	return data, err
}

func (l *LockGeter) clean(now int64) {
	if now-l.time < 5 {
		return
	}
	l.time = now
	l.caches.Range(func(key, value any) bool {
		var v = value.(*cacheItem)
		if v.time < now && !v.loading {
			v.cancel()
			l.caches.Delete(key)
		}
		return true
	})
}

// Get http data, the returned buffer is caller-owned and readonly.
func Get(url string) (*bytes.Buffer, error) {
	data, err := getBytes(url)
	if err != nil {
		return nil, err
	}
	return bytes.NewBuffer(bytes.Clone(data)), nil
}

// getBytes http data, the return value is immutable and can be held forever.
func getBytes(url string) ([]byte, error) {
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
	var buf = bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		buf.Reset()
		bufferPool.Put(buf)
		return nil, err
	}
	data := bytes.Clone(buf.Bytes())
	buf.Reset()
	bufferPool.Put(buf)
	return data, nil
}
