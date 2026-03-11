# auto-uooc-go

一个使用 Go 编写的 UOOC 视频进度自动上报命令行工具。

## 使用方法

见 [简略版教程](README_ez.md)

## 许可证说明

本仓库已 GPL-3.0 协议开源，详见 LICENSE 文件。

欢迎借鉴思路至其它项目，但必须在关于信息/版权处提及并链接该仓库。

## 功能概览

- 支持输入并保存浏览器 Cookie，调用用户信息接口校验登录状态。
- 根据课程 cid 拉取课程目录并发现视频任务。
- 只处理视频任务，不处理测验、作业、考试。
- 自动跳过已完成任务（finished=1）。
- 闯关模式课程中，遇到 code=600 会暂停后续任务发现，先执行已发现任务。
- 视频时长通过 go-ffprobe 获取。
- 支持 dry-run（只计算不实际提交）。
- 全局最多并发 3 个视频任务。
- 支持通过 CLI 独立修改配置。

## 构建

```bash
go build -o auto-uooc-go .
```

## 运行依赖

需要系统可用 ffprobe（通常由 ffmpeg 提供）。

- Ubuntu/Debian

```bash
sudo apt-get install ffmpeg
```

- macOS

```bash
brew install ffmpeg
```

- Windows

下载并安装 FFmpeg 后，确保 ffprobe.exe 可被程序找到（见下文优先级）。

## 命令说明

### 1. 登录与 Cookie 保存

```bash
./auto-uooc-go auth --cookie "k1=v1; k2=v2"
```

不传 --cookie 时会交互输入。

### 2. 配置管理

查看当前配置

```bash
./auto-uooc-go config --show
```

设置配置项

```bash
./auto-uooc-go config --report-interval 20 --speed-multiplier 2 --ffprobe-path /usr/bin/ffprobe
```

设置 Cookie

```bash
./auto-uooc-go config --cookie "k1=v1; k2=v2"
```

清空 ffprobe_path

```bash
./auto-uooc-go config --clear-ffprobe-path
```

### 3. 执行课程视频进度上报

```bash
./auto-uooc-go run --cid 1507587250
```

常用参数

- --dry-run
- --report-interval
- --speed-multiplier
- --ffprobe-path
- --cookie

示例

```bash
./auto-uooc-go run --cid 1507587250 --dry-run --report-interval 20 --speed-multiplier 2
```

## 进度推进规则

- 上报间隔 report_interval_sec，默认 20 秒。
- 速度倍率 speed_multiplier，默认 2。
- 每次上报推进秒数 = 上报间隔 × 速度倍率。
- 默认即每 20 秒上报一次，每次推进 40 秒。
- 上报位置会限制在视频尾部以内（video_length - 1）。
- 返回 finished 非 0 时停止该任务。

## 闯关模式与任务发现行为

- 任务按课程目录顺序发现并入队。
- 一旦某小节 getUnitLearn 返回 code=600（闯关限制），停止继续发现后续小节。
- 已入队任务会继续执行。
- 执行结束后会提醒先完成测验/前置任务，再次运行即可继续。

## 配置文件位置

配置使用 viper 读写，格式为 JSON。

写入优先级

1. 程序所在目录下 .config/config.json
2. 若程序目录不可写（如 /usr/bin），回退到标准配置目录

标准配置目录

- Linux/macOS
1. XDG_CONFIG_HOME/auto-uooc-go/config.json（若设置）
2. ~/.config/auto-uooc-go/config.json

- Windows
1. %APPDATA%/auto-uooc-go/config.json
2. UserConfigDir/auto-uooc-go/config.json（APPDATA 不可用时）

配置键

- cookie
- report_interval_sec
- speed_multiplier
- ffprobe_path

## ffprobe 查找优先级

1. 配置中的 ffprobe_path
2. 环境变量 FFPROBE_PATH
3. 程序执行目录扫描（与可执行文件同目录，文件名包含 ffprobe 且可执行）
4. 系统 PATH 中的 ffprobe

## 注意事项

- 仅处理视频任务。
- 无法获取视频时长的任务会被跳过并在汇总中显示。
- 非 Windows 平台会将配置文件权限设置为 0600。
