# 简略版教程

1. 在软件同目录/环境变量下准备ffprobe二进制(windows下zip内已预置，unix-like环境下安装ffmpeg即可)
2. 打开浏览器uooc网站，F12打开控制台，输入`document.cookie`，复制输出的cookies字符串
3. 运行`./auto-uooc-go auth`，输入cookies字符串
4. 运行`./auto-uooc-go run --cid 课程ID`，课程ID即课程页面URL中的数字部分
5. 就会自动开始上报进度。

## 获取课程ID(cid)

例如，课程链接为`https://www.uooc.net.cn/home/course/1507587250#/result`，则课程ID为`1507587250`。

## 修改配置

```bash
auto-uooc-go config [--show] [--cookie "k=v; ..."] [--report-interval 20] [--speed-multiplier 2] [--concurrency 3] [--ffprobe-path /usr/bin/ffprobe] [--clear-ffprobe-path]
```

```txt
  --show 显示当前配置
  --report-interval 上报间隔，单位秒，默认20
  --speed-multiplier 速度倍率，每次上报进度增加的秒数=上报间隔*速度倍率，默认2
  --concurrency 可以访问的视频任务并发数，默认3
```

1. 上报间隔(每多少秒上报一次进度) 默认20s
2. 速度倍率(每次上报进度加的秒数=上报间隔*速度倍率) 默认为2
3. ffprobe路径，不填写则尝试环境变量，其次尝试当前目录

## 默认效果

对于上述默认配置，示例课程的闯关模式，未答题时，第1大章共5小章7个视频开放，则会同时对前3个视频，每20秒上报一次进度，每次上报的进度增加40秒。

当目前没有需要完成的视频任务时，就会结束程序。
