# Go2TV - 无线投屏软件

Go2TV 是一个使用 Go 语言开发的无线投屏软件，支持搜索局域网中的 DLNA 设备并将 MP4 格式的电影投放到这些设备上。

## 功能特性

- 📱 自动搜索局域网中的 DLNA 投屏设备
- 🎬 支持 MP4 格式电影的无线投屏
- 🎨 简洁美观的图形用户界面
- 🚀 快速稳定的投屏体验

## 安装和运行

### 前提条件

- Go 1.24.2 或更高版本
- 安装 Fyne 所需的系统依赖
  - macOS: `xcode-select --install` 安装 Command Line Tools
  - Linux: 请参考 [Fyne 官方文档](https://developer.fyne.io/started/)
  - Windows: 一般情况下无需额外安装

### 获取源码

```bash
# 克隆仓库
git clone https://github.com/your-username/go2tv.git
cd go2tv
```

### 安装依赖

```bash
go mod tidy
```

### 运行程序

```bash
go run main.go
```

### 编译程序

```bash
go build -o go2tv main.go
```

## 使用方法

1. **搜索设备**：点击"搜索设备"按钮，程序会自动搜索局域网中的 DLNA 设备
2. **选择文件**：点击"选择文件"按钮，选择一个 MP4 格式的电影文件
3. **开始投屏**：从设备列表中选择一个设备，然后点击"开始投屏"按钮

## 项目结构

```
go2tv/
├── discovery/    # 设备发现相关代码
│   └── ssdp.go   # SSDP 协议实现，用于搜索 DLNA 设备
├── dlna/         # DLNA 通信相关代码
│   └── control.go # 控制 DLNA 设备的实现
├── server/       # 媒体服务器相关代码
│   └── media_server.go # 提供媒体文件的 HTTP 服务器
├── main.go       # 主程序入口和 UI 实现
├── go.mod        # Go 模块依赖声明
└── README.md     # 项目说明文档
```

## 技术说明

- **设备发现**：使用 SSDP (Simple Service Discovery Protocol) 协议搜索局域网中的 DLNA 设备
- **媒体控制**：通过 DLNA/UPnP 协议控制设备播放媒体内容
- **用户界面**：使用 Fyne 库构建跨平台的图形用户界面
- **媒体提供**：内置 HTTP 服务器，提供媒体文件的流式传输

## 注意事项

- 确保您的电脑和投屏设备连接在同一局域网内
- 某些 DLNA 设备可能需要特定的媒体格式或编码，本程序目前主要支持 MP4 格式
- 如果投屏失败，可以尝试重新搜索设备或检查网络连接

## 贡献

欢迎提交 Issue 或 Pull Request 来帮助改进这个项目！