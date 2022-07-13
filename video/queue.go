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
const chunk = 1024 * 60

var (
	queueManager = newdcQueueManager()
	httpProvider = request.NewLockGeter(time.Second * 5)
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
	ID       string
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

// dcQueueManager 维护所有datachannel到dcConnections里,每个datachannel对应一个dcQueue
func (q *dcQueueManager) send(d *webrtc.DataChannel, buffer *bufferTask) {
	ctx, cancel := context.WithCancel(context.Background())
	t, loaded := q.dcConnections.LoadOrStore(fmt.Sprintf("%d", d.ID()), &dcQueue{
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

func (q *dcQueueManager) quit(d *webrtc.DataChannel, id string, index []uint64) {
	v, ok := q.dcConnections.Load(fmt.Sprintf("%d", d.ID()))
	if !ok {
		return
	}
	dq := v.(*dcQueue)
	for _, i := range index {
		dq.rmTask(id, i)
	}
}

func (q *dcQueueManager) stats() map[string]*ItemStat {
	var res = map[string]*ItemStat{}
	q.dcConnections.Range(func(key, value interface{}) bool {
		v := value.(*dcQueue)
		res[key.(string)] = &ItemStat{
			ID:       fmt.Sprintf("%d", v.dc.ID()),
			Label:    v.dc.Label(),
			Buffered: v.dc.BufferedAmount(),
			State:    v.dc.ReadyState().String(),
			Tasks:    len(v.tasks),
		}
		return true
	})
	return res
}

// 如果任务队列中已有此任务,则忽略;任务队列中有多个不同的id组,因为对等的datachannel可以同时查询多个视频
func (d *dcQueue) addTask(buffer *bufferTask) {
	var exist = false
	d.lock.RLock()
	for _, item := range d.tasks {
		if buffer.id == item.id && buffer.index == item.index {
			exist = true
			buffer.cancel()
		}
	}
	d.lock.RUnlock()
	if exist {
		return
	}
	d.lock.Lock()
	d.tasks = append(d.tasks, buffer)
	d.lock.Unlock()
}

// get the first task from array
func (d *dcQueue) getTask() *bufferTask {
	d.lock.Lock()
	defer d.lock.Unlock()
	var l = len(d.tasks)
	if l < 1 {
		return nil
	}
	task := d.tasks[0]
	d.tasks = d.tasks[1:]
	return task
}

func (d *dcQueue) rmTask(id string, index uint64) {
	d.lock.Lock()
	defer d.lock.Unlock()
	var l = len(d.tasks)
	if l < 1 {
		return
	}
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

func (d *dcQueue) doTask(task *bufferTask) error {
	var buffers [][]byte
	select {
	case <-task.ctx.Done():
		return nil
	case <-d.ctx.Done():
		return nil
	default:
		// 因使用了缓存池,bs只读并且需尽快使用,等会过期将会被其他地方复用
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
			start  = time.Now()
		)
		// 为保障buffers尽快去消费,我们限定在5s内使用(但是底层Send仍会引用此数据,实际可能大于此时间),超过这个时间作废
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
					return err
				}
				var n = d.dc.BufferedAmount() / maxBufferedAmount
				if n < 1 {
					n = 1
				}
				time.Sleep(time.Millisecond * time.Duration(100*n))
			}
			if time.Since(start) > time.Second*5 {
				return nil
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
			if d.dc.ReadyState() != webrtc.DataChannelStateOpen {
				return
			}
			time.Sleep(time.Second)
			if d.dc.BufferedAmount() > 1024 {
				continue
			}
			task = d.getTask()
			if task == nil {
				time.Sleep(time.Second)
				continue
			}
			if err = d.doTask(task); err != nil {
				util.Log.Print(err)
			}
		}
	}
}

// 自定义分片协议,前端按照此协议组装
// [id, sn, i, n]
// 协议头|版本号|头部长度
// 二进制协议
// protocol 16位 0x7364
// version x0a1

func chunkHeader(id string, index uint64, i int, l int) []byte {
	var meta = fmt.Sprintf(`["%s",%d,%d,%d]`, id, index, i, l)
	var head = []byte{0x73, 0x64, 0xa1, uint8(len(meta))}
	return append(head, []byte(meta)...)
}

func splitBuffer(bs []byte) [][]byte {
	var (
		buffers = [][]byte{}
		start   = 0
		end     int
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
