


ws的任务是传输信令,和节点发现.

跟踪上线的用户,与上线的所有用户,发送P2P链接请求

最终维护所有P2P链接,被动接受datachannel数据,

监听到是下载分片请求,则立即下载并发送出去



ws.Loop 开始建立链接,监听消息


收到 offer 

收到 candidate

回复 answer

回复 candidate



====


收到 offer

回复 answer

收到 candidate

收到 candidate


主动连接端 

createDataChannel 



消息

ping

pong

对方发送 query

我们回复found

resolve

quit


message.buffer

buffer.recv

buffer

等, 
我们作为一个P2P节点,不向别人索要资源,只提供资源,只需要实现

监听 ping 回复 pong
监听到 pong 无需处理
监听到 query 回复 found
监听到 resolve 回复二进制内容消息

回复二进制内容消息为分片数据,50kb一分片
须由队列执行 发送 buffer.recv,算上头部30字节 共计 50KB + 30字节
头部格式为 `["id",i,n]`

i 为当前分片的需要,从0开始
n 为总分片数量
id 为形式 `vid:itag|index`

> vid 是video id
>
> itag 即为video 的itag
>
> index 为资源媒体sidx或cues的媒体分片



监听到 quit 则停止队列发送

设计一个队列发送器

创建队列,对数据切片发送

每个dc 维护一个队列
如果发送失败,则放弃本次发送

TODO total quit