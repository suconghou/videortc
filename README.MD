## videortc

like http://github.com/suconghou/videoproxy but works as a cdn cache node which use webrtc p2p network

### 工作流程

系统启动后与信令服务器( https://github.com/suconghou/signalserver )建立连接

建立连接成功收到`init`事件

如有其它在线用户,则`init`事件会携带已在线用户,其它已在线用户会收到此用户`online`事件

已在线用户会主动链接新`online`的用户,本端等待链接

连接使用WS交换信令信息.

链接建立后会建立DataChannel用于数据通信

DataChannel 发送文本消息(P2P分片查询协商)和二进制消息(媒体数据)


消息类型有

* ping
* pong
* 对方发送的`query`
* 我们回复的`found`
* 对方发来的`resolve`
* 对方发来的`quit`

等, 还有其他本系统不需要实现, 同 https://github.com/suconghou/libwebrtc

我们作为一个P2P节点,不向别人索要资源,只提供资源,只需要实现

* 监听 ping 回复 pong
* 监听到 pong 无需处理
* 监听到 query 分析是否可用 回复 found
* 监听到 resolve 回复二进制媒体消息

回复二进制媒体消息为分片数据,50kb一分片

由队列执行,前30字节为数据包header,算上头部30字节 共计 50KB + 30字节

头部格式为 `["id",i,n]`

> i 为当前分片的序号,从0开始
>
> n 为总分片数量
>
> id 为形式 `vid:itag|index`

> vid 是video id
>
> itag 即为video 的itag
>
> index 为资源媒体sidx或cues的媒体分片


* 监听到 quit 则会给队列发送消息,停止队列


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
> UPSTREAM 作为负载均衡器,反向代理其他服务, 例如 "https://video.feds.club" 多个可用;号隔开
>
> PUBLICIP 可选配置,需要形式 `ip:port` 或者`ip`的形式,内部使用`SetNAT1To1IPs`和`SetICEUDPMux`优化穿透
> 
> 负载策略为随机算法,当第一次回源出错时,会选择其他源再试一次,若没有其他源,也会再试一次
>
> 如需修改ICE Server需要在代码中修改

**工作模式**

需配置`ID`和`WS_ADDR`使用webrtc功能,否则退化为`videoproxy`(本项目构建于videoproxy之上)

如果开启`BASE_URL`,则会使用指定的服务解析服务,本节点可在国内部署.

否则,要国内部署,需使用`VIDEO_PROXY`配置代理

> 如果境外服务器,无任何环境变量即等同于`videoproxy`;配置`ID`和`WS_ADDR`可开启其rtc功能
>
> 如果在境内,需要配置`VIDEO_PROXY`使用代理,同`videoproxy`的代理配置;借助于此代理,配置`ID`和`WS_ADDR`可开启其rtc功能
>
> 如果没有代理,`BASE_URL`配合`ID`和`WS_ADDR`可在境内开启rtc功能;媒体解析接口因境内又没配置代理,将不可用,可配置`UPSTREAM`将接口负载均衡到其他服务上


## 缓存

系统中视频解析和下载等均包含内存缓存,并确保了并发时仅单个请求在重建缓存,使用量较大时,内存占用可能较多.

系统自动维护缓存和队列数据

接口`/status`查看运行状态

接口`/peers`查看p2p网络节点和链接状态, `/peers?t=video` 查看媒体缓存和队列信息

## docker

`docker pull suconghou/videortc:latest`

```bash
docker run --rm \
-it \
--name videortc \
-m 200m \
-p 6060:6060 \
-e ID="36位ID" \
-e WS_ADDR="wss://ws.feds.club/uid/" \
-e BASE_URL="https://video.feds.club/video" \
suconghou/videortc:latest
```


## 子项目

* https://github.com/suconghou/videoproxy
* https://github.com/suconghou/mediaindex
* https://github.com/suconghou/youtubevideoparser
