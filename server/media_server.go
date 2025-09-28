package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"GoCastify/transcoder"
)

// 常量定义
const (
	defaultBufferSize    = 32 * 1024  // 32KB 缓冲区
	httpReadTimeout      = 30 * time.Second
	httpWriteTimeout     = 30 * time.Second
	httpIdleTimeout      = 120 * time.Second
	serverShutdownTimeout = 5 * time.Second
)

// MediaServer 提供媒体文件的HTTP服务器
type MediaServer struct {
	httpServer *http.Server
	port       int
	mediaPath  string
	isRunning  bool
	mu         sync.Mutex
	transcoder *transcoder.Transcoder
}

// NewMediaServer 创建一个新的媒体服务器
func NewMediaServer(port int) (*MediaServer, error) {
	// 初始化转码器
	transcoder, err := transcoder.NewTranscoder()
	if err != nil {
		return nil, fmt.Errorf("初始化转码器失败: %w", err)
	}

	return &MediaServer{
		port:       port,
		transcoder: transcoder,
	}, nil
}

// Start 启动媒体服务器
func (ms *MediaServer) Start(mediaPath string) (string, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.isRunning {
		// 如果服务器已经在运行，检查媒体路径是否相同
		if ms.mediaPath == mediaPath {
			// 路径相同，直接返回当前服务器URL
			return ms.GetServerURL(), nil
		}
		// 路径不同，先停止服务器
		ms.Stop()
	}

	// 设置媒体路径
	ms.mediaPath = mediaPath

	// 创建HTTP处理器
	handler := http.NewServeMux()
	// 处理根路径，提供媒体文件的目录列表
		handler.HandleFunc("/", ms.handleMediaRequest)

		// 创建HTTP服务器

	// 创建HTTP服务器
	ms.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", ms.port),
		Handler:      handler,
		ReadTimeout:  httpReadTimeout,
		WriteTimeout: httpWriteTimeout,
		IdleTimeout:  httpIdleTimeout,
	}

	// 在后台启动服务器
	go func() {
		log.Printf("媒体服务器启动在端口: %d\n", ms.port)
		if err := ms.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("媒体服务器错误: %v\n", err)
			ms.mu.Lock()
			ms.isRunning = false
			ms.mu.Unlock()
		}
	}()

	// 标记服务器为运行状态
	ms.isRunning = true

	// 返回服务器的URL
	return ms.GetServerURL(), nil
}

// Stop 停止媒体服务器
func (ms *MediaServer) Stop() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if !ms.isRunning || ms.httpServer == nil {
		return nil
	}

	// 创建一个有超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer cancel()

	// 关闭服务器
	err := ms.httpServer.Shutdown(ctx)
	if err != nil {
		log.Printf("媒体服务器关闭错误: %v\n", err)
		return err
	}

	// 清理转码器资源
	if ms.transcoder != nil {
		if cleanupErr := ms.transcoder.Cleanup(); cleanupErr != nil {
			log.Printf("转码器清理错误: %v\n", cleanupErr)
		}
	}

	ms.isRunning = false
	log.Println("媒体服务器已停止")
	return nil
}

// GetServerURL 获取媒体服务器的URL
func (ms *MediaServer) GetServerURL() string {
	// 获取本地IP地址
	ip := getLocalIP()
	if ip == "" {
		ip = "localhost"
	}

	return fmt.Sprintf("http://%s:%d", ip, ms.port)
}

// handleMediaRequest 处理媒体文件请求
func (ms *MediaServer) handleMediaRequest(w http.ResponseWriter, r *http.Request) {
	// 记录请求
	log.Printf("收到请求: %s %s\n", r.Method, r.URL.Path)

	// 获取请求的文件路径
	filePath := filepath.Join(ms.mediaPath, r.URL.Path)

	// 检查文件是否存在
	if !ms.fileExists(filePath) {
		http.NotFound(w, r)
		return
	}

	// 设置CORS头，允许跨域请求
	ms.setCORSHeaders(w)

	// 处理OPTIONS请求
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// 检查是否需要转码
	supported, needTranscode := transcoder.IsSupportedFormat(filePath)
	if !supported {
		http.Error(w, "不支持的媒体格式", http.StatusUnsupportedMediaType)
		log.Printf("不支持的媒体格式: %s\n", filePath)
		return
	}

	// 如果不需要转码，直接提供文件
	if !needTranscode {
		ms.serveFileEfficiently(w, r, filePath)
		return
	}

	// 处理需要转码的文件
	ms.handleTranscodedMedia(w, r, filePath)
}

// fileExists 检查文件是否存在
func (ms *MediaServer) fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		log.Printf("检查文件失败: %v\n", err)
	}
	return err == nil
}

// setCORSHeaders 设置CORS响应头
func (ms *MediaServer) setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")
}

// handleTranscodedMedia 处理需要转码的媒体文件
func (ms *MediaServer) handleTranscodedMedia(w http.ResponseWriter, r *http.Request, filePath string) {
	// 检查是否启用了转码功能
	if ms.transcoder == nil {
		http.Error(w, "转码功能未初始化", http.StatusInternalServerError)
		log.Printf("转码功能未初始化\n")
		return
	}

	// 检查FFmpeg是否可用
	if !transcoder.CheckFFmpeg() {
		http.Error(w, "未找到FFmpeg，无法转码。请先安装FFmpeg。", http.StatusInternalServerError)
		log.Printf("未找到FFmpeg，无法转码\n")
		return
	}

	// 获取URL中的字幕轨道和音频轨道参数
	subtitleTrackIndex := ms.parseTrackIndex(r.URL.Query().Get("subtitle"), "字幕")
	audioTrackIndex := ms.parseTrackIndex(r.URL.Query().Get("audio"), "音频")

	// 转码文件
	transcodedFile, err := ms.transcoder.TranscodeToMp4(filePath, subtitleTrackIndex, audioTrackIndex)
	if err != nil {
		http.Error(w, fmt.Sprintf("转码失败: %v", err), http.StatusInternalServerError)
		log.Printf("转码失败: %v\n", err)
		return
	}

	// 高效提供转码后的文件
	ms.serveFileEfficiently(w, r, transcodedFile)
}

// parseTrackIndex 解析轨道索引参数
func (ms *MediaServer) parseTrackIndex(param string, trackType string) int {
	if param == "" {
		return -1
	}

	index, err := strconv.Atoi(param)
	if err != nil {
		log.Printf("无效的%s轨道索引: %s, 使用默认值(-1)", trackType, param)
		return -1
	}

	return index
}

// serveFileEfficiently 高效地提供文件服务，支持范围请求和缓冲传输
func (ms *MediaServer) serveFileEfficiently(w http.ResponseWriter, req *http.Request, filePath string) {
	// 检查文件是否存在
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("文件不存在: %v", err), http.StatusNotFound)
		return
	}

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("无法打开文件: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// 设置内容类型
	contentType := "application/octet-stream"
	ext := strings.ToLower(filepath.Ext(filePath))
	supportedMimeTypes := map[string]string{
		".mp4":  "video/mp4",
		".mkv":  "video/x-matroska",
		".avi":  "video/x-msvideo",
		".mov":  "video/quicktime",
		".mp3":  "audio/mpeg",
		".aac":  "audio/aac",
		".flac": "audio/flac",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
	}
	if mimeType, exists := supportedMimeTypes[ext]; exists {
		contentType = mimeType
	}
	w.Header().Set("Content-Type", contentType)

	// 文件大小
	fileSize := fileInfo.Size()

	// 支持范围请求
	rangeHeader := req.Header.Get("Range")

	// 如果没有范围请求，使用http.ServeContent提供文件
	if rangeHeader == "" {
		w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
		w.Header().Set("Accept-Ranges", "bytes")
		http.ServeContent(w, req, fileInfo.Name(), fileInfo.ModTime(), file)
		return
	}

	// 处理范围请求
	ms.handleRangeRequest(w, req, file, fileSize)
}

// handleRangeRequest 处理HTTP范围请求
func (ms *MediaServer) handleRangeRequest(w http.ResponseWriter, req *http.Request, file *os.File, fileSize int64) {
	// 设置接受范围头
	w.Header().Set("Accept-Ranges", "bytes")

	// 简单的范围请求处理逻辑
	start := int64(0)
	end := int64(fileSize - 1)

	// 解析范围请求
	rangeHeader := req.Header.Get("Range")
	if len(rangeHeader) > 6 && rangeHeader[:6] == "bytes=" {
		parts := strings.Split(rangeHeader[6:], "-")
		if len(parts) > 0 && parts[0] != "" {
			if s, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
				start = s
			}
		}
		if len(parts) > 1 && parts[1] != "" {
			if e, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				end = e
			}
		}
	}

	// 验证范围
	if start < 0 || start >= fileSize {
		http.Error(w, "无效的范围请求", http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if end < start || end >= fileSize {
		end = fileSize - 1
	}

	// 计算要读取的字节数
	length := end - start + 1

	// 设置部分内容响应头
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.WriteHeader(http.StatusPartialContent)

	// 创建有限的读取器
	reader := io.NewSectionReader(file, start, length)

	// 使用缓冲区提高传输效率
	buffer := make([]byte, defaultBufferSize)
	io.CopyBuffer(w, reader, buffer)
}

// getLocalIP 获取本地IP地址
func getLocalIP() string {
	// 获取所有网络接口
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("获取网络接口失败: %v\n", err)
		return ""
	}

	// 遍历所有网络接口
	for _, iface := range interfaces {
		// 跳过无效的网络接口
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		// 获取接口的IP地址
		addresses, err := iface.Addrs()
		if err != nil {
			log.Printf("获取接口地址失败: %v\n", err)
			continue
		}

		// 遍历所有IP地址
		for _, addr := range addresses {
			// 解析IP地址
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.IsLoopback() {
				continue
			}

			// 检查是否为IPv4地址
			ipv4 := ipNet.IP.To4()
			if ipv4 != nil {
				return ipv4.String()
			}
		}
	}

	return ""
}