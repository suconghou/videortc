package video

import (
	"context"
	"fmt"
	"sync"
	"time"
	"videortc/request"
	"videortc/util"

	"github.com/pion/webrtc/v3"
)

const maxBufferedAmount uint64 = 1024 * 1024 // 1 MB
const chunk = 51200

var (
	queueManager = newdcQueueManager()
	httpProvider = request.NewLockGeter(time.Hour)
)

type dcQueueManager struct {
	dcConnections sync.Map
}

type dcQueue struct {
	dc     *webrtc.DataChannel
	tasks  []*bufferTask
	lock   *sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// ItemStat for queue status
type ItemStat struct {
	ID       *uint16
	Label    string
	State    string
	Tasks    int
	Buffered uint64
}

func newdcQueueManager() *dcQueueManager {
	return &dcQueueManager{
		dcConnections: sync.Map{},
	}
}

func (q *dcQueueManager) send(d *webrtc.DataChannel, buffer *bufferTask) {
	ctx, cancel := context.WithCancel(context.Background())
	t, loaded := q.dcConnections.LoadOrStore(d.ID(), &dcQueue{
		d,
		[]*bufferTask{},
		&sync.RWMutex{},
		ctx,
		cancel,
	})
	v := t.(*dcQueue)
	v.addTask(buffer)
	if loaded {
		// 原本已存在此队列,此队列必然已是运行状态,本次无需操作
		return
	}
	// 否则,是我们本次新建的队列,我们需要启动此队列
	go func() {
		v.loopTask()
		q.clean()
	}()
}

// 检查中断的DataChannel,清理任务
func (q *dcQueueManager) clean() {
	q.dcConnections.Range(func(key, value interface{}) bool {
		var item = value.(*dcQueue)
		if item.dc.ReadyState() == webrtc.DataChannelStateClosed || item.dc.ReadyState() == webrtc.DataChannelStateClosing {
			item.cancel()
			q.dcConnections.Delete(key)
		}
		return true
	})
}

func (q *dcQueueManager) quit(did *uint16, id string, index uint64) {
	v, ok := q.dcConnections.Load(did)
	if !ok {
		return
	}
	v.(*dcQueue).quit(id, index)
}

func (q *dcQueueManager) stats() map[uint16]*ItemStat {
	var res = map[uint16]*ItemStat{}
	q.dcConnections.Range(func(key, value interface{}) bool {
		var id uint16 = *(key.(*uint16))
		v := value.(*dcQueue)
		res[id] = &ItemStat{
			ID:       v.dc.ID(),
			Label:    v.dc.Label(),
			Buffered: v.dc.BufferedAmount(),
			State:    v.dc.ReadyState().String(),
			Tasks:    len(v.tasks),
		}
		return true
	})
	return res
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
	defer d.lock.Unlock()
	if id == "" && index == 0 {
		// cancel all task
		for i, x := range d.tasks {
			x.cancel()
			d.tasks[i] = nil
		}
		d.tasks = []*bufferTask{}
		return
	}

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
}

func (d *dcQueue) quit(id string, index uint64) {
	d.rmTask(id, index)
}

func (d *dcQueue) doTask(task *bufferTask) error {
	var buffers [][]byte
	select {
	case <-task.ctx.Done():
		return nil
	case <-d.ctx.Done():
		return nil
	default:
		bs, err := httpProvider.Get(task.target)
		if err != nil {
			return err
		}
		buffers = splitBuffer(bs)
	}
	select {
	case <-task.ctx.Done():
		return nil
	case <-d.ctx.Done():
		return nil
	default:
		var (
			err    error
			l      = len(buffers)
			i      int
			buffer []byte
		)
		for i, buffer = range buffers {
			select {
			case <-task.ctx.Done():
				return nil
			case <-d.ctx.Done():
				return nil
			default:
				if d.dc.ReadyState() != webrtc.DataChannelStateOpen {
					return nil
				}
				err = d.dc.Send(append(chunkHeader(task.id, task.index, i, l), buffer...))
				if err != nil {
					break
				} else {
					var n = d.dc.BufferedAmount() / maxBufferedAmount
					if n < 1 {
						n = 1
					}
					time.Sleep(time.Millisecond * time.Duration(100*n))
				}
			}
		}
		return err
	}
}

func (d *dcQueue) loopTask() {
	var task *bufferTask
	var err error
	for {
		select {
		case <-d.ctx.Done():
			return
		default:
			var n = d.dc.BufferedAmount() / maxBufferedAmount
			if n < 1 {
				n = 1
			}
			time.Sleep(time.Second * time.Duration(n))
			if d.dc.ReadyState() == webrtc.DataChannelStateClosed || d.dc.ReadyState() == webrtc.DataChannelStateClosing {
				return
			}
			task = d.getTask()
			if task == nil {
				time.Sleep(time.Second)
				continue
			}
			if err = d.doTask(task); err != nil {
				util.Log.Print(err)
			}
			task = nil
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

func splitBuffer(bs []byte) [][]byte {
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
	return buffers
}
