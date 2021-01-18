package route

import (
	"net/http"
	"regexp"

	"github.com/suconghou/videoproxy/route"
)

// 路由定义
type routeInfo struct {
	Reg     *regexp.Regexp
	Handler func(http.ResponseWriter, *http.Request, []string) error
}

// Route for all route
var Route = route.Route
