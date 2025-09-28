# GoCastify

GoCastify是一款基于Go语言开发的DLNA投屏工具，允许用户轻松将本地媒体文件（视频、音频）投射到支持DLNA的设备上，如智能电视、音响等。

## 功能特性

- 📺 支持视频文件投屏播放
- 🎵 支持音频文件投屏播放
- 📝 支持字幕文件选择和投屏
- 🔍 自动发现局域网内的DLNA设备
- 🎯 支持多音轨选择
- 💻 简洁直观的用户界面
- 🌐 内置HTTP媒体服务器
- ⚡ 高效的媒体转码功能（基于FFmpeg）

## 技术栈

- **后端语言**: Go
- **UI框架**: fyne.io
- **媒体转码**: FFmpeg
- **网络协议**: DLNA/UPnP
- **构建工具**: Go Modules

## 安装方法

### 前提条件

- 安装Go 1.18或更高版本
- 安装FFmpeg（用于媒体转码）

### 编译安装

```bash
# 克隆仓库
git clone https://github.com/cshbaoo/GoCastify.git
cd GoCastify

# 编译项目
go build

# 运行应用
./GoCastify
```

## 使用说明

1. 启动GoCastify应用程序
2. 应用会自动搜索局域网内的DLNA设备
3. 点击"选择文件"按钮，选择要投屏的媒体文件
4. 可选：点击"选择音轨"按钮，选择要使用的音频轨道
5. 可选：点击"选择字幕"按钮，选择要使用的字幕文件
6. 选择目标DLNA设备
7. 点击"开始投屏"按钮开始播放

## 项目架构

GoCastify采用了清晰的分层架构和接口设计，主要组件包括：

### 核心模块

- **app/** - 应用程序的核心逻辑和状态管理
- **interfaces/** - 定义了所有模块间交互的接口，确保模块解耦
- **types/** - 定义了共享的数据类型，确保全项目类型统一

### 功能模块

- **discovery/** - 负责DLNA设备发现，实现了`interfaces.DeviceDiscoverer`接口
- **dlna/** - 提供DLNA设备控制功能，实现了`interfaces.DLNAController`接口
- **server/** - 内置HTTP媒体服务器，实现了`interfaces.MediaServer`接口
- **transcoder/** - 媒体转码功能，基于FFmpeg，实现了`interfaces.MediaTranscoder`接口
- **ui/** - 用户界面实现

### 项目结构

```
GoCastify/
├── app/
│   └── app.go     # 应用主逻辑实现
├── discovery/
│   └── ssdp.go    # SSDP协议实现，DLNA设备发现
├── dlna/
│   └── control.go # DLNA设备控制功能
├── interfaces/
│   └── interfaces.go # 核心接口定义
├── server/
│   └── media_server.go # HTTP媒体服务器实现
├── transcoder/
│   └── transcoder.go # 基于FFmpeg的转码实现
├── types/
│   └── types.go   # 共享数据类型定义
├── ui/
│   └── ui.go      # 用户界面实现
├── go.mod         # Go模块定义
├── go.sum         # 依赖版本锁定
└── main.go        # 程序入口
```

## 接口设计

项目采用了清晰的接口设计，主要接口包括：

### DLNAController
- `PlayMediaWithContext(ctx context.Context, mediaURL string) error` - 带上下文支持的媒体播放函数
- `GetDeviceInfo() types.DeviceInfo` - 获取设备信息

### MediaServer
- `Start(mediaDir string) (string, error)` - 启动媒体服务器，返回服务器URL
- `Stop() error` - 停止媒体服务器
- `ServeHTTP(w http.ResponseWriter, r *http.Request)` - 处理HTTP请求

### MediaTranscoder
- `GetSubtitleTracks(filePath string) ([]types.SubtitleTrack, error)` - 获取媒体文件中的字幕轨道信息
- `GetAudioTracks(filePath string) ([]types.AudioTrack, error)` - 获取媒体文件中的音频轨道信息
- `TranscodeToMp4(inputFile string, subtitleTrackIndex int, audioTrackIndex int) (string, error)` - 将媒体文件转码为MP4格式
- `StreamTranscode(inputFile string, subtitleTrackIndex int, audioTrackIndex int) (string, error)` - 实时流式转码
- `Cleanup() error` - 清理临时文件和资源

### DeviceDiscoverer
- `StartSearchWithContext(ctx context.Context, onDeviceFound func(types.DeviceInfo)) error` - 开始搜索DLNA设备
- `GetDevices() []types.DeviceInfo` - 获取已发现的设备列表

## 最佳实践

1. **使用带上下文的方法** - 优先使用带`WithContext`后缀的方法，它们支持超时控制和取消操作

2. **设备发现** - 使用`DeviceDiscoverer`接口进行设备搜索，避免直接使用具体实现

3. **媒体服务器** - 媒体服务器通过依赖注入接收转码器，便于测试和替换实现

4. **类型安全** - 全项目使用统一的`types.DeviceInfo`类型表示设备信息

## 开发说明

### 依赖管理

项目使用Go Modules进行依赖管理：

```bash
# 安装依赖
go mod tidy

# 更新依赖
go get -u
```

### 开发流程

1. Fork并克隆仓库
2. 创建新的功能分支
3. 实现功能或修复bug
4. 确保代码可以正常编译
5. 提交代码并创建Pull Request

## 注意事项

- **弃用警告** - `StartCasting`方法已弃用，请使用`StartCastingWithContext`方法以获得更好的控制和取消功能
- **性能优化** - 设备搜索采用并发处理和信号量限制，避免过多的并发请求
- **资源管理** - 确保正确调用`Cleanup`方法释放转码器资源
- **错误处理** - 所有关键操作都有详细的错误处理和日志记录

## 许可证

本项目采用自定义开源许可证，允许个人学习和非商业用途使用，但禁止任何商业用途。详情请查看LICENSE文件