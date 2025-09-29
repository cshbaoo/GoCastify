package app

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"GoCastify/dlna"
	"GoCastify/server"
	"GoCastify/transcoder"
	"GoCastify/types"
)

// 常量定义
const (
	defaultMediaServerPort   = 8080
	dialogWidth              = 600
	dialogHeight             = 450
	progressDialogWidth      = 400
	progressDialogHeight     = 200
)

// createCustomProgressDialog 创建自定义进度对话框
func createCustomProgressDialog(title, message string, parent fyne.Window) dialog.Dialog {
	// 创建标题和消息标签
	titleLabel := widget.NewLabel(title)
	messageLabel := widget.NewLabel(message)
	messageLabel.Wrapping = fyne.TextWrapWord

	// 创建进度条（默认隐藏）
	progressBar := widget.NewProgressBar()
	progressBar.Hide()

	// 创建无限加载动画
	infiniteBar := widget.NewProgressBarInfinite()

	// 组织内容
	content := container.NewVBox(
		layout.NewSpacer(),
		container.NewHBox(layout.NewSpacer(), titleLabel, layout.NewSpacer()),
		container.NewHBox(layout.NewSpacer(), messageLabel, layout.NewSpacer()),
		layout.NewSpacer(),
		container.NewHBox(layout.NewSpacer(), infiniteBar, layout.NewSpacer()),
		container.NewHBox(layout.NewSpacer(), progressBar, layout.NewSpacer()),
		layout.NewSpacer(),
	)

	// 创建自定义对话框
	dlg := dialog.NewCustom(title, "取消", content, parent)
	dlg.Resize(fyne.NewSize(progressDialogWidth, progressDialogHeight))

	// 返回对话框
	return dlg
}

// App 表示整个应用程序的状态和功能
type App struct {
	Window                fyne.Window
	FyneApp               fyne.App
	Devices               []types.DeviceInfo
	SelectedDeviceIndex   int
	MediaFile             string
	MediaServer           *server.MediaServer
	FFmpegAvailable       bool
	SubtitleTracks        []types.SubtitleTrack
	SelectedSubtitleIndex int
	AudioTracks           []types.AudioTrack
	SelectedAudioIndex    int
	SearchCancel          context.CancelFunc
	DeviceList            *widget.List
	RecentPath            string // 最近访问的文件路径
}

// NewApp 创建一个新的应用程序实例
func NewApp(fyneApp fyne.App, window fyne.Window) (*App, error) {
	// 创建转码器
	transcoderInstance, _ := transcoder.NewTranscoder()

	// 创建媒体服务器
	mediaServer := server.NewMediaServer(defaultMediaServerPort, transcoderInstance)

	// 检查FFmpeg是否可用
	ffmpegAvailable := transcoder.CheckFFmpeg()

	return &App{
		Window:                window,
		FyneApp:               fyneApp,
		Devices:               []types.DeviceInfo{},
		SelectedDeviceIndex:   -1,
		MediaFile:             "",
		MediaServer:           mediaServer,
		FFmpegAvailable:       ffmpegAvailable,
		SubtitleTracks:        []types.SubtitleTrack{},
		SelectedSubtitleIndex: -1,
		AudioTracks:           []types.AudioTrack{},
		SelectedAudioIndex:    -1,
	}, nil
}

// CreateSearchContext 创建一个用于设备搜索的上下文
func (app *App) CreateSearchContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// StartCastingWithContext 开始投屏操作（带上下文支持）
func (app *App) StartCastingWithContext(ctx context.Context, progress dialog.Dialog) error {
	selectedDevice := app.Devices[app.SelectedDeviceIndex]
	log.Printf("连接设备: %s, 地址: %s\n", selectedDevice.FriendlyName, selectedDevice.Location)

	// 创建设备控制器
	controller, err := dlna.NewDeviceControllerWithContext(ctx, selectedDevice.Location)
	if err != nil {
		return fmt.Errorf("创建设备控制器失败: %w", err)
	}

	// 获取文件所在目录
	mediaDir := filepath.Dir(app.MediaFile)
	fileName := filepath.Base(app.MediaFile)

	// 启动媒体服务器并获取媒体文件的HTTP URL
	var serverURL string
	if app.MediaServer != nil {
		serverURL, err = app.MediaServer.Start(mediaDir)
		if err != nil {
			return fmt.Errorf("启动媒体服务器失败: %w", err)
		}
	} else {
		// 如果没有媒体服务器，使用本地文件路径（这可能只在某些设备上工作）
		serverURL = "file://" + mediaDir
	}

	// 构建媒体文件的完整URL
	mediaURL := app.buildMediaURL(serverURL, fileName)
	log.Printf("媒体文件URL: %s\n", mediaURL)

	// 播放媒体
	err = controller.PlayMediaWithContext(ctx, mediaURL)
	if err != nil {
		return fmt.Errorf("投屏失败: %w", err)
	}

	log.Printf("投屏成功: %s\n", filepath.Base(app.MediaFile))
	return nil
}

// StartCasting 开始投屏操作
// 注意：此方法已弃用，请使用带上下文支持的StartCastingWithContext方法
//
// Deprecated: Use StartCastingWithContext instead for better control and cancellation
func (app *App) StartCasting(progress dialog.Dialog) {
	// 创建一个带有超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 执行带上下文的投屏操作
	err := app.StartCastingWithContext(ctx, progress)
	if err != nil {
		log.Printf("投屏操作失败: %v\n", err)
		dialog.ShowError(err, app.Window)
	} else {
		dialog.ShowInformation("成功", "投屏成功！\n媒体文件正在通过HTTP服务器提供", app.Window)
	}

	// 关闭加载对话框
	progress.Hide()
}

// SelectAudio 打开音频选择对话框
func (app *App) SelectAudio(audioLabel *widget.Label) {
	if app.MediaFile == "" {
		dialog.ShowInformation("提示", "请先选择一个媒体文件", app.Window)
		return
	}

	// 检查FFmpeg是否可用
	if !transcoder.CheckFFmpeg() {
		dialog.ShowInformation("转码功能不可用", "未找到FFmpeg，无法提取音频信息。\n请安装FFmpeg以支持音频选择功能。", app.Window)
		return
	}

	// 显示加载对话框
	progress := createCustomProgressDialog("正在获取音频信息", "请稍候...", app.Window)
	progress.Show()

	// 在后台获取音频轨道信息
	go func() {
		transcoderInstance, err := transcoder.NewTranscoder()
		if err != nil {
			log.Printf("创建转码器失败: %v\n", err)
			dialog.ShowError(err, app.Window)
			progress.Hide()
			return
		}

		// 获取音频轨道信息
		audioTracks, err := transcoderInstance.GetAudioTracks(app.MediaFile)
		if err != nil {
			log.Printf("获取音频信息失败: %v\n", err)
			dialog.ShowError(err, app.Window)
			progress.Hide()
			return
		}

		// 保存音频轨道信息
		app.AudioTracks = []types.AudioTrack{}
		for _, track := range audioTracks {
			app.AudioTracks = append(app.AudioTracks, types.AudioTrack{
				Index:     track.Index,
				Language:  track.Language,
				Title:     track.Title,
				CodecName: track.CodecName,
				IsDefault: track.IsDefault,
			})
		}

		// 更新UI
		app.Window.Content().Refresh()

		// 关闭进度对话框
		progress.Hide()

		// 如果没有音频轨道
		if len(audioTracks) == 0 {
			dialog.ShowInformation("音频信息", "当前视频文件中未找到音频轨道", app.Window)
			app.SelectedAudioIndex = -1
			audioLabel.SetText("音轨: 无")
			audioLabel.Refresh()
			return
		}

		// 创建音频轨道选择列表
		audioList := widget.NewList(
			func() int {
				return len(audioTracks) + 1 // +1 表示"默认音轨"选项
			},
			func() fyne.CanvasObject {
				// 创建更美观的列表项，符合苹果UI设计风格
				item := widget.NewLabel("音频选项")
				item.TextStyle = fyne.TextStyle{}
				item.Wrapping = fyne.TextTruncate
				// 使用容器来设置最小尺寸
				return container.NewMax(item)
			},
			func(id widget.ListItemID, obj fyne.CanvasObject) {
				container := obj.(*fyne.Container)
				label := container.Objects[0].(*widget.Label)
				label.TextStyle = fyne.TextStyle{}
				label.Wrapping = fyne.TextTruncate
				if id == 0 {
					label.SetText("默认音轨")
				} else {
					track := audioTracks[id-1]
					title := track.Title
					if title == "" {
						title = "未命名音频"
					}
					if track.Language != "" {
						title += " (" + track.Language + ")"
					}
					if track.CodecName != "" {
						title += " [" + track.CodecName + "]"
					}
					if track.IsDefault {
						title += " [默认]"
						label.TextStyle = fyne.TextStyle{Bold: true} // 默认轨道使用粗体，符合苹果突出显示的风格
					}
					label.SetText(fmt.Sprintf("%d: %s", id-1, title))
				}
			},
		)

		// 创建带有内边距的容器来包裹列表，确保内容有足够的空间
		paddedList := container.NewPadded(
			container.NewMax(
				audioList,
				// 添加占位元素来确保对话框有足够的大小
				fyne.NewContainerWithLayout(
					layout.NewGridLayout(1),
					widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{}), // 占位元素
					widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{}),
					widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{}),
					widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{}),
					widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{}),
				),
			),
		)

		// 创建说明标签，符合苹果UI的清晰性原则
		descriptionLabel := widget.NewLabel("请选择您想要使用的音频轨道：")
		descriptionLabel.TextStyle = fyne.TextStyle{Bold: true} // 标题使用粗体

		// 创建符合macOS设计规范的对话框布局
		dialogContent := container.NewVBox(
			container.NewPadded(descriptionLabel),
			widget.NewSeparator(), // 分隔线增强视觉层次
			container.NewPadded(paddedList),
		)

		// 创建带有取消按钮的自定义对话框，符合macOS UI设计标准
		audioDialog := dialog.NewCustomConfirm("选择音频轨道", "确定", "取消", dialogContent, func(confirmed bool) {}, app.Window)
		// 调整对话框大小以符合macOS设计风格
		audioDialog.Resize(fyne.NewSize(dialogWidth, dialogHeight))

		// 修复重复显示的问题
		// audioDialog.Show() 会在后面的OnSelected设置完成后调用

		// 设置列表选择事件
		audioList.OnSelected = func(id widget.ListItemID) {
			if id == 0 {
				app.SelectedAudioIndex = -1
				audioLabel.SetText("音轨: 默认")
			} else {
				app.SelectedAudioIndex = audioTracks[id-1].Index
				title := audioTracks[id-1].Title
				if title == "" {
					title = "未命名音频"
				}
				if audioTracks[id-1].Language != "" {
					title += " (" + audioTracks[id-1].Language + ")"
				}
				audioLabel.SetText(fmt.Sprintf("音轨: %s", title))
			}
			audioLabel.Refresh()
			audioDialog.Hide()
		}

		// 显示音频选择对话框
		audioDialog.Show()
	}()
}

// SelectSubtitle 打开字幕选择对话框
func (app *App) SelectSubtitle(subtitleLabel *widget.Label) {
	if app.MediaFile == "" {
		dialog.ShowInformation("提示", "请先选择一个媒体文件", app.Window)
		return
	}

	// 检查FFmpeg是否可用
	if !transcoder.CheckFFmpeg() {
		dialog.ShowInformation("转码功能不可用", "未找到FFmpeg，无法提取字幕信息。\n请安装FFmpeg以支持字幕选择功能。", app.Window)
		return
	}

	// 显示加载对话框
	progress := createCustomProgressDialog("处理中...", "正在提取视频中的字幕信息", app.Window)
	progress.Show()

	// 在后台提取字幕信息
	go func() {
		// 创建转码器实例
		transcoderInstance, err := transcoder.NewTranscoder()
		if err != nil {
			log.Printf("创建转码器失败: %v\n", err)
			dialog.ShowError(err, app.Window)
			progress.Hide()
			return
		}

		// 获取字幕轨道信息
		subtitleTracks, err := transcoderInstance.GetSubtitleTracks(app.MediaFile)
		if err != nil {
			log.Printf("获取字幕信息失败: %v\n", err)
			dialog.ShowError(err, app.Window)
			progress.Hide()
			return
		}

		// 保存字幕轨道信息
		app.SubtitleTracks = []types.SubtitleTrack{}
		for _, track := range subtitleTracks {
			app.SubtitleTracks = append(app.SubtitleTracks, types.SubtitleTrack{
				Index:     track.Index,
				Language:  track.Language,
				Title:     track.Title,
				IsDefault: track.IsDefault,
			})
		}

		// 更新UI
		app.Window.Content().Refresh()

		// 关闭进度对话框
		progress.Hide()

		// 如果没有字幕轨道
		if len(subtitleTracks) == 0 {
			dialog.ShowInformation("字幕信息", "当前视频文件中未找到字幕轨道", app.Window)
			app.SelectedSubtitleIndex = -1
			subtitleLabel.SetText("字幕: 无")
			subtitleLabel.Refresh()
			return
		}

		// 创建字幕选择列表，优化UI以符合苹果设计标准
		subtitleList := widget.NewList(
			func() int {
				return len(subtitleTracks) + 1 // +1 表示"无字幕"选项
			},
			func() fyne.CanvasObject {
				// 创建更美观的列表项，符合苹果UI设计风格
				item := widget.NewLabel("字幕选项")
				item.TextStyle = fyne.TextStyle{}
				item.Wrapping = fyne.TextTruncate
				// 使用容器来设置最小尺寸
				return container.NewMax(item)
			},
			func(id widget.ListItemID, obj fyne.CanvasObject) {
				container := obj.(*fyne.Container)
				label := container.Objects[0].(*widget.Label)
				label.TextStyle = fyne.TextStyle{}
				label.Wrapping = fyne.TextTruncate
				if id == 0 {
					label.SetText("无字幕")
				} else {
					track := subtitleTracks[id-1]
					title := track.Title
					if title == "" {
						title = "未命名字幕"
					}
					if track.Language != "" {
						title += " (" + track.Language + ")"
					}
					if track.IsDefault {
						title += " [默认]"
						label.TextStyle = fyne.TextStyle{Bold: true} // 默认轨道使用粗体，符合苹果突出显示的风格
					}
					label.SetText(fmt.Sprintf("%d: %s", id-1, title))
				}
			},
		)

		// 创建自定义列表的容器，增加内边距
		paddedList := container.NewPadded(subtitleList)

		// 创建符合macOS设计规范的对话框布局
		label := widget.NewLabel("请选择您想要使用的字幕轨道")
		label.Alignment = fyne.TextAlignCenter
		label.TextStyle = fyne.TextStyle{Bold: true}
		dialogContent := container.NewVBox(
			container.NewPadded(label),
			widget.NewSeparator(), // 分隔线增强视觉层次
			container.NewPadded(paddedList),
		)

		// 创建带有取消按钮的自定义对话框，符合macOS UI设计标准
		subtitleDialog := dialog.NewCustomConfirm("选择字幕轨道", "确定", "取消", dialogContent, func(confirmed bool) {}, app.Window)
		// 调整对话框大小以符合macOS设计风格
		subtitleDialog.Resize(fyne.NewSize(dialogWidth, dialogHeight))

		// 设置列表选择事件
		subtitleList.OnSelected = func(id widget.ListItemID) {
			if id == 0 {
				app.SelectedSubtitleIndex = -1
				subtitleLabel.SetText("字幕: 无")
			} else {
				app.SelectedSubtitleIndex = subtitleTracks[id-1].Index
				title := subtitleTracks[id-1].Title
				if title == "" {
					title = "未命名字幕"
				}
				if subtitleTracks[id-1].Language != "" {
					title += " (" + subtitleTracks[id-1].Language + ")"
				}
				subtitleLabel.SetText(fmt.Sprintf("字幕: %s", title))
			}
			subtitleLabel.Refresh()
			subtitleDialog.Hide()
		}

		// 显示字幕选择对话框
		subtitleDialog.Show()
	}()
}

// buildMediaURL 构建媒体文件的完整URL，包括可选的字幕和音频参数
func (app *App) buildMediaURL(serverURL, fileName string) string {
	mediaURL := serverURL + "/" + fileName

	// 添加查询参数
	params := []string{}
	if app.SelectedSubtitleIndex >= 0 {
		params = append(params, "subtitle="+strconv.Itoa(app.SelectedSubtitleIndex))
	}
	if app.SelectedAudioIndex >= 0 {
		params = append(params, "audio="+strconv.Itoa(app.SelectedAudioIndex))
	}

	// 拼接查询参数
	if len(params) > 0 {
		mediaURL += "?" + strings.Join(params, "&")
	}

	return mediaURL
}

// Cleanup 清理应用资源
func (app *App) Cleanup() {
	// 停止设备搜索
	if app.SearchCancel != nil {
		app.SearchCancel()
		app.SearchCancel = nil
	}

	// 停止媒体服务器
	if app.MediaServer != nil {
		if err := app.MediaServer.Stop(); err != nil {
			log.Printf("停止媒体服务器时出错: %v\n", err)
		}
		app.MediaServer = nil
	}

	// 清空设备列表
	app.Devices = nil
	app.SelectedDeviceIndex = -1
}
