package transcoder

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Transcoder 处理媒体格式检测和转码
type Transcoder struct {
	// 缓存转码结果以提高性能
	transcodingCache map[string]string
	cacheMutex       sync.Mutex
	// 缓存过期时间
	cacheExpiry map[string]time.Time
	// 临时文件存储
	tempDir string
	// 字幕轨道信息缓存
	subtitleTracks map[string][]SubtitleTrack
	subtitleMutex  sync.Mutex
	// 音频轨道信息缓存
	audioTracks map[string][]AudioTrack
	audioMutex  sync.Mutex
	// 限制并发转码任务数量
	maxConcurrentTranscodes int
	semaphore              chan struct{}
}

// SubtitleTrack 表示媒体文件中的字幕轨道信息
type SubtitleTrack struct {
	Index     int
	Language  string
	Title     string
	IsDefault bool
}

// AudioTrack 表示媒体文件中的音频轨道信息
type AudioTrack struct {
	Index     int
	Language  string
	Title     string
	CodecName string
	IsDefault bool
}

// NewTranscoder 创建一个新的转码器
func NewTranscoder() (*Transcoder, error) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "go2tv_transcode_")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}

	// 获取CPU核心数，设置最大并发转码任务数
	cpuCount := runtime.NumCPU()
	maxConcurrentTranscodes := cpuCount / 2
	if maxConcurrentTranscodes < 1 {
		maxConcurrentTranscodes = 1
	}

	return &Transcoder{
		transcodingCache:        make(map[string]string),
		cacheMutex:              sync.Mutex{},
		cacheExpiry:             make(map[string]time.Time),
		tempDir:                 tempDir,
		subtitleTracks:          make(map[string][]SubtitleTrack),
		subtitleMutex:           sync.Mutex{},
		audioTracks:             make(map[string][]AudioTrack),
		audioMutex:              sync.Mutex{},
		maxConcurrentTranscodes: maxConcurrentTranscodes,
		semaphore:               make(chan struct{}, maxConcurrentTranscodes),
	},
		nil
}

// 支持的可转码格式
var supportedTranscodeFormats = map[string]bool{
	".mkv": true,
	".avi": true,
	".wmv": true,
	".flv": true,
	".mov": true,
	".mpg": true,
	".mpeg": true,
	".webm": true,
}

// 需要转码的音频格式
var needTranscodeAudioFormats = map[string]bool{
	"dts": true,
	"ac3": true,
}

// IsSupportedFormat 检查文件格式是否受支持（原生支持或可转码）
func IsSupportedFormat(filePath string) (bool, bool) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".mp4" || ext == ".m4v" {
		// MP4格式通常原生支持
		return true, false
	}
	// 检查是否支持转码
	if supportedTranscodeFormats[ext] {
		return true, true
	}
	return false, false
}

// CheckFFmpeg 检查系统是否安装了FFmpeg
func CheckFFmpeg() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// GetMediaInfo 获取媒体文件信息
func (t *Transcoder) GetMediaInfo(filePath string) (map[string]string, error) {
	if !CheckFFmpeg() {
		return nil, fmt.Errorf("未找到FFmpeg，请先安装FFmpeg")
	}

	cmd := exec.Command("ffprobe", 
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name,width,height,duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("获取媒体信息失败: %w, 输出: %s", err, string(output))
	}

	info := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) > 0 {
		info["video_codec"] = lines[0]
	}
	if len(lines) > 1 {
		info["width"] = lines[1]
	}
	if len(lines) > 2 {
		info["height"] = lines[2]
	}
	if len(lines) > 3 {
		info["duration"] = lines[3]
	}

	// 检查音频编解码器
	audioCmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath)
	audioOutput, err := audioCmd.CombinedOutput()
	if err == nil {
		audioCodec := strings.TrimSpace(string(audioOutput))
		info["audio_codec"] = audioCodec
	}

	return info, nil
}

// GetSubtitleTracks 获取媒体文件中的字幕轨道信息
func (t *Transcoder) GetSubtitleTracks(filePath string) ([]SubtitleTrack, error) {
	// 检查缓存中是否已有该文件的字幕轨道信息
	t.subtitleMutex.Lock()
	cachedTracks, exists := t.subtitleTracks[filePath]
	t.subtitleMutex.Unlock()

	if exists {
		return cachedTracks, nil
	}

	if !CheckFFmpeg() {
		return nil, fmt.Errorf("未找到FFmpeg，请先安装FFmpeg")
	}

	// 使用ffprobe获取所有字幕轨道信息
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "s",
		"-show_entries", "stream=index:stream_tags=language,title",
		"-of", "csv=p=0",
		filePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("获取字幕轨道信息失败: %w, 输出: %s", err, string(output))
	}

	tracks := []SubtitleTrack{}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		// 解析CSV格式的输出: index,language,title
		parts := strings.Split(line, ",")
		track := SubtitleTrack{
			IsDefault: false,
		}

		if len(parts) > 0 {
			index, err := strconv.Atoi(parts[0])
			if err == nil {
				track.Index = index
			}
		}

		if len(parts) > 1 {
			track.Language = parts[1]
		}

		if len(parts) > 2 {
			track.Title = parts[2]
		}

		// 如果是第一条字幕轨道，默认为选中
		if len(tracks) == 0 && (track.Language == "zh" || track.Language == "zh-CN" || track.Language == "en") {
			track.IsDefault = true
		}

		tracks = append(tracks, track)
	}

	// 缓存字幕轨道信息
	t.subtitleMutex.Lock()
	t.subtitleTracks[filePath] = tracks
	t.subtitleMutex.Unlock()

	return tracks, nil
}

// GetAudioTracks 获取媒体文件中的音频轨道信息
func (t *Transcoder) GetAudioTracks(filePath string) ([]AudioTrack, error) {
	// 检查缓存中是否已有该文件的音频轨道信息
	t.audioMutex.Lock()
	cachedTracks, exists := t.audioTracks[filePath]
	t.audioMutex.Unlock()

	if exists {
		return cachedTracks, nil
	}

	if !CheckFFmpeg() {
		return nil, fmt.Errorf("未找到FFmpeg，请先安装FFmpeg")
	}

	// 使用ffprobe获取所有音频轨道信息
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "a",
		"-show_entries", "stream=index:stream_tags=language,title:stream=codec_name",
		"-of", "csv=p=0",
		filePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("获取音频轨道信息失败: %w, 输出: %s", err, string(output))
	}

	tracks := []AudioTrack{}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		// 解析CSV格式的输出: index,language,title,codec_name
		parts := strings.Split(line, ",")
		track := AudioTrack{
			IsDefault: false,
		}

		if len(parts) > 0 {
			index, err := strconv.Atoi(parts[0])
			if err == nil {
				track.Index = index
			}
		}

		if len(parts) > 1 {
			track.Language = parts[1]
		}

		if len(parts) > 2 {
			track.Title = parts[2]
		}

		if len(parts) > 3 {
			track.CodecName = parts[3]
		}

		// 如果是第一条音频轨道，默认为选中
		if len(tracks) == 0 {
			track.IsDefault = true
		}

		tracks = append(tracks, track)
	}

	// 缓存音频轨道信息
	t.audioMutex.Lock()
	t.audioTracks[filePath] = tracks
	t.audioMutex.Unlock()

	return tracks, nil
}

// TranscodeToMp4 将媒体文件转码为MP4格式
// 支持实时流输出，适用于投屏场景
func (t *Transcoder) TranscodeToMp4(inputFile string, subtitleTrackIndex int, audioTrackIndex int) (string, error) {
	// 生成带字幕和音频索引的缓存键
	cacheKey := fmt.Sprintf("%s_subtitle_%d_audio_%d", inputFile, subtitleTrackIndex, audioTrackIndex)

	// 检查是否已有缓存的转码结果
	if outputFile, valid := t.getCachedOutput(cacheKey); valid {
		log.Printf("使用缓存的转码结果: %s", outputFile)
		return outputFile, nil
	}

	if !CheckFFmpeg() {
		return "", fmt.Errorf("未找到FFmpeg，请先安装FFmpeg")
	}

	// 限制并发转码任务数量
	t.semaphore <- struct{}{}
	defer func() {
		<-t.semaphore
	}()

	// 创建输出文件路径
	baseName := strings.TrimSuffix(filepath.Base(inputFile), filepath.Ext(inputFile))
	suffix := ""
	if subtitleTrackIndex >= 0 {
		suffix += fmt.Sprintf("_sub%d", subtitleTrackIndex)
	}
	if audioTrackIndex >= 0 {
		suffix += fmt.Sprintf("_audio%d", audioTrackIndex)
	}
	outputFile := filepath.Join(t.tempDir, fmt.Sprintf("%s_transcoded%s.mp4", baseName, suffix))

	// 获取媒体信息
	mediaInfo, err := t.GetMediaInfo(inputFile)
	if err != nil {
		return "", fmt.Errorf("获取媒体信息失败: %w", err)
	}

	// 构建FFmpeg转码参数，优化性能
	args := t.buildOptimizedTranscodeArgs(inputFile, outputFile, mediaInfo, subtitleTrackIndex, audioTrackIndex)

	// 记录转码开始时间
	startTime := time.Now()
	log.Printf("开始转码文件: %s 到 %s", inputFile, outputFile)

	// 执行转码命令
	cmd := exec.Command("ffmpeg", args...)

	// 捕获标准输出和错误输出
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("创建标准输出管道失败: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("创建标准错误管道失败: %w", err)
	}

	// 启动命令
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动转码命令失败: %w", err)
	}

	// 并发读取输出
	go func() {
		io.Copy(os.Stdout, stdout)
	}()

	go func() {
		// 处理FFmpeg输出，提取进度信息
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				output := string(buf[:n])
				// 这里可以添加进度解析逻辑
				if strings.Contains(output, "time=") {
					// 简单进度记录
					log.Printf("转码中: %s", strings.TrimSpace(output))
				}
			}
			if err != nil {
				break
			}
		}
	}()

	// 等待转码完成
	if err := cmd.Wait(); err != nil {
		// 转码失败，删除输出文件
		os.Remove(outputFile)
		return "", fmt.Errorf("转码失败: %w", err)
	}

	// 计算转码耗时
	duration := time.Since(startTime)
	log.Printf("转码完成，耗时: %v", duration)

	// 缓存转码结果，设置24小时过期
	t.cacheMutex.Lock()
	t.transcodingCache[cacheKey] = outputFile
	t.cacheExpiry[cacheKey] = time.Now().Add(24 * time.Hour)
	t.cacheMutex.Unlock()

	return outputFile, nil
}

// StreamTranscode 实时流式转码（适合大型文件）
func (t *Transcoder) StreamTranscode(inputFile string, subtitleTrackIndex int, audioTrackIndex int) (string, error) {
	// 这个方法将实现实时流式转码
	// 对于大型文件，我们可以创建一个临时HTTP端点，通过FFmpeg实时转码并流式传输
	// 此处简化实现，实际项目中需要更复杂的处理

	// 检查FFmpeg是否安装
	if !CheckFFmpeg() {
		return "", fmt.Errorf("未找到FFmpeg，请先安装FFmpeg")
	}

	// 在这个简化版本中，我们直接使用TranscodeToMp4
	// 实际项目中应该实现真正的流式转码
	return t.TranscodeToMp4(inputFile, subtitleTrackIndex, audioTrackIndex)
}

// 提供一个向后兼容的无字幕版本
func (t *Transcoder) TranscodeToMp4NoSubtitle(inputFile string, audioTrackIndex int) (string, error) {
	return t.TranscodeToMp4(inputFile, -1, audioTrackIndex)
}

// 提供一个向后兼容的无字幕版本的StreamTranscode
func (t *Transcoder) StreamTranscodeNoSubtitle(inputFile string, audioTrackIndex int) (string, error) {
	return t.StreamTranscode(inputFile, -1, audioTrackIndex)
}

// Cleanup 清理临时文件和资源
func (t *Transcoder) Cleanup() error {
	t.cacheMutex.Lock()
	defer t.cacheMutex.Unlock()

	// 清理过期缓存
	t.cleanupExpiredCache()

	// 清理缓存记录
	t.transcodingCache = make(map[string]string)
	t.cacheExpiry = make(map[string]time.Time)

	// 清理临时目录
	if t.tempDir != "" {
		if err := os.RemoveAll(t.tempDir); err != nil {
			return fmt.Errorf("清理临时目录失败: %w", err)
		}
		t.tempDir = ""
	}

	return nil
}

// 内部方法: 获取缓存的输出文件路径，如果缓存有效返回路径和true
func (t *Transcoder) getCachedOutput(cacheKey string) (string, bool) {
	t.cacheMutex.Lock()
	defer t.cacheMutex.Unlock()

	// 先清理过期缓存
	t.cleanupExpiredCache()

	// 检查是否有缓存
	cachedOutput, exists := t.transcodingCache[cacheKey]
	if !exists {
		return "", false
	}

	// 检查缓存文件是否存在
	if _, err := os.Stat(cachedOutput); err != nil {
		// 缓存文件不存在，移除缓存记录
		delete(t.transcodingCache, cacheKey)
		delete(t.cacheExpiry, cacheKey)
		return "", false
	}

	return cachedOutput, true
}

// 内部方法: 清理过期的缓存
func (t *Transcoder) cleanupExpiredCache() {
	now := time.Now()
	expiredKeys := []string{}

	// 找出所有过期的缓存键
	for key, expiry := range t.cacheExpiry {
		if now.After(expiry) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	// 删除过期的缓存文件和记录
	for _, key := range expiredKeys {
		if filePath, exists := t.transcodingCache[key]; exists {
			// 尝试删除文件，但不处理错误
			os.Remove(filePath)
			// 移除缓存记录
			delete(t.transcodingCache, key)
		}
		delete(t.cacheExpiry, key)
	}
}

// 内部方法: 构建优化的转码参数
func (t *Transcoder) buildOptimizedTranscodeArgs(inputFile, outputFile string, mediaInfo map[string]string, subtitleTrackIndex, audioTrackIndex int) []string {
	// 基本参数：高质量、快速启动（适合流式传输）
	args := []string{
		"-i", inputFile,
		"-c:v", "h264", // 使用H.264视频编码
		"-preset", "ultrafast", // 最快的编码速度
		"-crf", "28", // 较低的质量但更快的编码
		"-profile:v", "main", // 兼容性更好的配置
		"-level", "4.0",
		"-movflags", "+faststart", // 快速启动，适合流式传输
		"-threads", strconv.Itoa(runtime.NumCPU()), // 使用多核加速
		"-hide_banner", // 减少输出信息
		"-loglevel", "warning", // 只显示警告和错误
	}

	// 构建映射参数
	args = append(args, "-map", "0:v:0") // 视频流

	// 如果指定了音频轨道，使用指定的轨道
	if audioTrackIndex >= 0 {
		args = append(args, "-map", fmt.Sprintf("0:a:%d", audioTrackIndex)) // 选择的音频轨道
	} else {
		args = append(args, "-map", "0:a?")  // 所有音频流（如果有）
	}

	// 如果指定了字幕轨道，添加字幕处理参数
	if subtitleTrackIndex >= 0 {
		args = append(args, "-map", fmt.Sprintf("0:s:%d", subtitleTrackIndex)) // 选择的字幕轨道
		args = append(args, "-c:s", "mov_text") // 转换字幕为MP4兼容格式
		args = append(args, "-disposition:s:0", "default") // 设置为默认字幕
	}

	// 检查是否需要转码音频
	audioCodec, audioExists := mediaInfo["audio_codec"]
	if audioExists && needTranscodeAudioFormats[strings.ToLower(audioCodec)] {
		// 转码为更通用的AAC格式
		args = append(args, "-c:a", "aac", "-b:a", "128k")
	} else {
		// 复制音频流，节省资源
		args = append(args, "-c:a", "copy")
	}

	// 添加输出文件
	args = append(args, outputFile)

	return args
}

// GetTempDir 获取临时目录路径
func (t *Transcoder) GetTempDir() string {
	return t.tempDir
}