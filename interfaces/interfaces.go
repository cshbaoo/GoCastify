package interfaces

import (
	"context"
	"net/http"
	"GoCastify/types"
)

// DLNAController DLNA设备控制接口
type DLNAController interface {
	// PlayMediaWithContext 带上下文支持的媒体播放函数
	PlayMediaWithContext(ctx context.Context, mediaURL string) error
	// GetDeviceInfo 获取设备信息
	GetDeviceInfo() types.DeviceInfo
}

// MediaServer 媒体服务器接口
type MediaServer interface {
	// Start 启动媒体服务器，返回服务器URL
	Start(mediaDir string) (string, error)
	// Stop 停止媒体服务器
	Stop() error
	// ServeHTTP 处理HTTP请求
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// MediaTranscoder 媒体转码器接口
type MediaTranscoder interface {
	// GetSubtitleTracks 获取媒体文件中的字幕轨道信息
	GetSubtitleTracks(filePath string) ([]types.SubtitleTrack, error)
	// GetAudioTracks 获取媒体文件中的音频轨道信息
	GetAudioTracks(filePath string) ([]types.AudioTrack, error)
	// TranscodeToMp4 将媒体文件转码为MP4格式
	TranscodeToMp4(inputFile string, subtitleTrackIndex int, audioTrackIndex int) (string, error)
	// StreamTranscode 实时流式转码
	StreamTranscode(inputFile string, subtitleTrackIndex int, audioTrackIndex int) (string, error)
	// Cleanup 清理临时文件和资源
	Cleanup() error
}

// DeviceDiscoverer 设备发现接口
type DeviceDiscoverer interface {
	// StartSearchWithContext 开始搜索DLNA设备
	StartSearchWithContext(ctx context.Context, onDeviceFound func(types.DeviceInfo)) error
	// GetDevices 获取已发现的设备列表
	GetDevices() []types.DeviceInfo
}

// LoggerFactory 日志工厂接口
type LoggerFactory interface {
	// GetLogger 获取指定名称的日志记录器
	GetLogger(name string) Logger
}

// Logger 日志接口
type Logger interface {
	// Debug 记录调试信息
	Debug(format string, args ...interface{})
	// Info 记录普通信息
	Info(format string, args ...interface{})
	// Warn 记录警告信息
	Warn(format string, args ...interface{})
	// Error 记录错误信息
	Error(format string, args ...interface{})
}