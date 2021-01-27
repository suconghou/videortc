package proxy

import (
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"videortc/util"
)

var (
	upstreams     []*url.URL
	upstreamCount int
	bytePool      = &sync.Pool{
		New: func() interface{} {
			return make([]byte, 32*1024)
		},
	}
	httpBufferPool = &bytesBuffer{bytePool}
)

type bytesBuffer struct {
	pool *sync.Pool
}

func (b *bytesBuffer) Get() []byte {
	return b.pool.Get().([]byte)
}

func (b *bytesBuffer) Put(data []byte) {
	b.pool.Put(data)
}

// Handle proxy http request
func Handle(w http.ResponseWriter, r *http.Request) {
	var origin = r.URL
	curr, next := getUpstream()
	errorHandler := func(w http.ResponseWriter, r *http.Request, err error) {
		util.Log.Printf("%s : %s", r.URL.String(), err)
		modify := func(res *http.Response) error {
			res.Header.Del("Set-Cookie")
			return nil
		}
		serve(getDirector(next, origin), modify, nil, w, r)
	}
	serve(getDirector(curr, origin), modifyResponse, errorHandler, w, r)
}

func serve(director func(*http.Request), modifyResponse func(*http.Response) error, errorHandler func(http.ResponseWriter, *http.Request, error), w http.ResponseWriter, r *http.Request) {
	proxy := &httputil.ReverseProxy{
		Director:       director,
		FlushInterval:  time.Second,
		BufferPool:     httpBufferPool,
		ModifyResponse: modifyResponse,
		ErrorHandler:   errorHandler,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		},
	}
	proxy.ServeHTTP(w, r)
}

func getDirector(u *url.URL, origin *url.URL) func(*http.Request) {
	return func(req *http.Request) {
		req.Host = u.Host
		req.URL.Path = singleJoiningSlash(u.Path, origin.Path)
		req.URL.Host = u.Host
		req.URL.Scheme = u.Scheme
		req.Header.Del("Cookie")
		req.Header["X-Forwarded-For"] = nil
	}
}

func modifyResponse(res *http.Response) error {
	if !(res.StatusCode == http.StatusOK || res.StatusCode == http.StatusNotModified) {
		return fmt.Errorf("error status %s", res.Status)
	}
	res.Header.Del("Set-Cookie")
	return nil
}

func getUpstream() (*url.URL, *url.URL) {
	if upstreamCount < 1 {
		upstreams, upstreamCount = initUpstream()
	}
	if upstreamCount == 1 {
		return upstreams[0], upstreams[0]
	}
	var n = rand.Intn(upstreamCount)
	var curr = upstreams[n]
	var next = upstreams[(n+1)%upstreamCount]
	return curr, next
}

func initUpstream() ([]*url.URL, int) {
	var urls = []*url.URL{}
	for _, str := range strings.Split(os.Getenv("UPSTREAM"), ";") {
		if str != "" {
			u, err := url.Parse(str)
			if err != nil {
				util.Log.Print(err)
				continue
			}
			urls = append(urls, u)
		}
	}
	var l = len(urls)
	if l < 1 {
		util.Log.Fatal("invalid upstream")
	}
	return urls, l
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	if aslash && bslash {
		return a + b[1:]
	}
	if !aslash && !bslash {
		return a + "/" + b
	}
	return a + b
}
