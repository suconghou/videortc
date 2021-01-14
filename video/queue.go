package video

import (
	"context"
	"fmt"
	"sync"
	"time"
	"videortc/util"

	"github.com/pion/webrtc/v3"
)

var (
	queueManager = newdcQueueManager()
)

type dcQueueManager struct {
	dcConnections map[*uint16]*dcQueue
	lock          *sync.RWMutex
}

type dcQueue struct {
	dc     *webrtc.DataChannel
	tasks  []*bufferTask
	lock   *sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

func newdcQueueManager() *dcQueueManager {
	return &dcQueueManager{
		dcConnections: map[*uint16]*dcQueue{},
		lock:          &sync.RWMutex{},
	}
}

func (q *dcQueueManager) send(d *webrtc.DataChannel, buffer *bufferTask) {
	var did = d.ID()
	q.lock.RLock()
	conn := q.dcConnections[did]
	q.lock.RUnlock()
	if conn == nil {
		ctx, cancel := context.WithCancel(context.Background())
		conn = &dcQueue{
			d,
			[]*bufferTask{},
			&sync.RWMutex{},
			ctx,
			cancel,
		}
		q.lock.Lock()
		q.dcConnections[did] = conn
		q.lock.Unlock()
		go func() {
			conn.loopTask()
			q.clean()
		}()
	}
	conn.addTask(buffer)
}

func (q *dcQueueManager) clean() {
	q.lock.Lock()
	for key, item := range q.dcConnections {
		if item.dc.ReadyState() == webrtc.DataChannelStateClosed || item.dc.ReadyState() == webrtc.DataChannelStateClosing {
			item.cancel()
			delete(q.dcConnections, key)
		}
	}
	q.lock.Unlock()
}

func (q *dcQueueManager) quit(did *uint16, id string, index uint64) {
	q.lock.RLock()
	conn := q.dcConnections[did]
	q.lock.RUnlock()
	if conn == nil {
		return
	}
	conn.quit(id, index)
}

func (d *dcQueue) addTask(buffer *bufferTask) {
	d.lock.Lock()
	d.tasks = append(d.tasks, buffer)
	d.lock.Unlock()
}

// get the first task from array
func (d *dcQueue) getTask() *bufferTask {
	var l = len(d.tasks)
	if l < 1 {
		return nil
	}
	d.lock.Lock()
	task := d.tasks[0]
	d.tasks = d.tasks[1:]
	d.lock.Unlock()
	return task
}

func (d *dcQueue) rmTask(id string, index uint64) {
	var l = len(d.tasks)
	if l < 1 {
		return
	}
	d.lock.Lock()
	i := 0 // output index
	for _, x := range d.tasks {
		if x.id == id && x.index == index {
			x.cancel()
		} else {
			// copy and increment index
			d.tasks[i] = x
			i++
		}
	}
	// Prevent memory leak by erasing truncated values
	// (not needed if values don't contain pointers, directly or indirectly)
	for j := i; j < len(d.tasks); j++ {
		d.tasks[j] = nil
	}
	d.tasks = d.tasks[:i]
	d.lock.Unlock()
}

func (d *dcQueue) quit(id string, index uint64) {
	d.rmTask(id, index)
}

func (d *dcQueue) doTask(task *bufferTask) error {
	select {
	case <-task.ctx.Done():
		return nil
	default:
		var err error
		var l = len(task.buffers)
		for i, buffer := range task.buffers {
			select {
			case <-task.ctx.Done():
				return nil
			default:
				if d.dc.ReadyState() != webrtc.DataChannelStateOpen {
					return nil
				}
				// TODO flow control
				err = d.dc.Send(append(chunkHeader(task.id, task.index, i, l), buffer...))
				if err != nil {
					break
				}
			}
		}
		return err
	}
}

func (d *dcQueue) loopTask() {
	for {
		select {
		case <-d.ctx.Done():
			return
		default:
			if d.dc.ReadyState() == webrtc.DataChannelStateClosed || d.dc.ReadyState() == webrtc.DataChannelStateClosing {
				return
			}
			task := d.getTask()
			if task == nil {
				time.Sleep(time.Second)
				continue
			}
			if err := d.doTask(task); err != nil {
				util.Log.Print(err)
			}
		}
	}
}

// 自定义分片协议,前端按照此协议组装,header头必须30字符
// ["id",i,l]
// id = vid:itag|index
func chunkHeader(id string, index uint64, i int, l int) []byte {
	var header = fmt.Sprintf(`["%s|%d",%d,%d]`, id, index, i, l)
	return []byte(fmt.Sprintf("%-30s", header))
}
