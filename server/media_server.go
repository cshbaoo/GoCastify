package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"go2tv/transcoder"
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
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 记录请求
		log.Printf("收到请求: %s %s\n", r.Method, r.URL.Path)

		// 获取请求的文件路径
		filePath := filepath.Join(mediaPath, r.URL.Path)

		// 检查文件是否存在
		_, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		} else if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			log.Printf("检查文件失败: %v\n", err)
			return
		}

		// 设置CORS头，允许跨域请求
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")

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
			http.ServeFile(w, r, filePath)
			return
		}

		// 需要转码，检查是否启用了转码功能
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

			// 获取URL中的字幕轨道参数
		subtitleTrackIndex := -1
		subtitleParam := r.URL.Query().Get("subtitle")
		if subtitleParam != "" {
			var err error
			subtitleTrackIndex, err = strconv.Atoi(subtitleParam)
			if err != nil {
				log.Printf("无效的字幕轨道索引: %s, 使用默认值(-1)", subtitleParam)
				subtitleTrackIndex = -1
			}
		}

		// 获取URL中的音频轨道参数
		audioTrackIndex := -1
		audioParam := r.URL.Query().Get("audio")
		if audioParam != "" {
			var err error
			audioTrackIndex, err = strconv.Atoi(audioParam)
			if err != nil {
				log.Printf("无效的音频轨道索引: %s, 使用默认值(-1)", audioParam)
				audioTrackIndex = -1
			}
		}

		// 对于需要转码的文件，我们提供转码后的临时文件
		transcodedFile, err := ms.transcoder.TranscodeToMp4(filePath, subtitleTrackIndex, audioTrackIndex)
		if err != nil {
			http.Error(w, fmt.Sprintf("转码失败: %v", err), http.StatusInternalServerError)
			log.Printf("转码失败: %v\n", err)
			return
		}

		// 提供转码后的文件
		http.ServeFile(w, r, transcodedFile)
	})

	// 创建HTTP服务器
	ms.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", ms.port),
		Handler: handler,
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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