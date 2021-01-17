
系统启动后与WS建立连接

建立连接成功收到`init`事件

如有其它在线用户,则`init`事件将会返回,其它已在线用户会收到此用户`online`事件

已在线用户会主动链接新`online`的用户,本端等待链接

连接使用WS交换信令信息.

链接建立后会建立DataChannel用于数据通信

DataChannel 发送文本消息(P2P分片查询协商)和二进制消息(媒体数据)


消息类型有

ping

pong

对方发送的`query`

我们回复的`found`

对方发来的`resolve`

对方发来的`quit`

等, 还有其他本系统不需要实现, 同 https://github.com/suconghou/libwebrtc

我们作为一个P2P节点,不向别人索要资源,只提供资源,只需要实现

监听 ping 回复 pong
监听到 pong 无需处理
监听到 query 分析是否可用 回复 found
监听到 resolve 回复二进制媒体消息

回复二进制媒体消息为分片数据,50kb一分片
由队列执行,前30字节为数据包header,算上头部30字节 共计 50KB + 30字节
头部格式为 `["id",i,n]`

i 为当前分片的需要,从0开始
n 为总分片数量
id 为形式 `vid:itag|index`

> vid 是video id
>
> itag 即为video 的itag
>
> index 为资源媒体sidx或cues的媒体分片


监听到 quit 则会给队列发送消息,停止队列


## 配置

ID 为形式为`zznj1q6h-2hmf-1fmc-2ajh-20mx2hxk1r6r`的36个字符唯一ID

环境变量

> ID 配置peer的唯一ID
> 
> WS_ADDR 信令服务器 , 例如"wss://ws.feds.club/uid/"
>
> VIDEO_PROXY 同 https://github.com/suconghou/videoproxy 的VIDEO_PROXY配置项,
>
> BASE_URL 媒体解析服务器, 例如 "https://video.feds.club/video" 需要支持url range
>

如果开启`BASE_URL`,则会使用指定的服务解析服务,本节点可在国内部署.
否则,要国内部署,需使用`VIDEO_PROXY`配置代理


设计一种并发限制器

对指定的函数调用,当上次调用未执行完,时,下次调用被阻塞,等待上次调用执行完,返回上次调用的执行结果

自定义一个Locker

`AddICECandidate` 的调用,必须确保`RemoteDescription`已被set




