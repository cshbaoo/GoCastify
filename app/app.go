package app

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"go2tv/discovery"
	"go2tv/dlna"
	"go2tv/server"
	"go2tv/transcoder"
)

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

// App 表示整个应用程序的状态和功能
type App struct {
	Window              fyne.Window
	Devices             []discovery.DeviceInfo
	SelectedDeviceIndex int
	MediaFile           string
	MediaServer         *server.MediaServer
	FFmpegAvailable     bool
	SubtitleTracks      []SubtitleTrack
	SelectedSubtitleIndex int
	AudioTracks         []AudioTrack
	SelectedAudioIndex  int
	SearchCancel        context.CancelFunc
	DeviceList          *widget.List
}

// NewApp 创建一个新的应用程序实例
func NewApp(window fyne.Window) (*App, error) {
	// 创建媒体服务器
	mediaServer, err := server.NewMediaServer(8080)
	if err != nil {
		log.Printf("创建媒体服务器失败: %v\n", err)
		// 继续运行，因为没有媒体服务器仍然可以提供基本功能
		mediaServer = nil
	}

	// 检查FFmpeg是否可用
	ffmpegAvailable := transcoder.CheckFFmpeg()

	return &App{
		Window:              window,
		Devices:             []discovery.DeviceInfo{},
		SelectedDeviceIndex: -1,
		MediaFile:           "",
		MediaServer:         mediaServer,
		FFmpegAvailable:     ffmpegAvailable,
		SubtitleTracks:      []SubtitleTrack{},
		SelectedSubtitleIndex: -1,
		AudioTracks:         []AudioTrack{},
		SelectedAudioIndex:  -1,
	},
		nil
}

// CreateSearchContext 创建一个用于设备搜索的上下文
func (app *App) CreateSearchContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// StartCasting 开始投屏操作
func (app *App) StartCasting(progress *dialog.ProgressDialog) {
	selectedDevice := app.Devices[app.SelectedDeviceIndex]
	log.Printf("连接设备: %s, 地址: %s\n", selectedDevice.FriendlyName, selectedDevice.Location)

	// 创建设备控制器
	controller, err := dlna.NewDeviceController(selectedDevice.Location)
	if err != nil {
		log.Printf("创建设备控制器失败: %v\n", err)
		dialog.ShowError(err, app.Window)
		progress.Hide()
		return
	}

	// 获取文件所在目录
	mediaDir := filepath.Dir(app.MediaFile)
	fileName := filepath.Base(app.MediaFile)

	// 启动媒体服务器并获取媒体文件的HTTP URL
	var serverURL string
	if app.MediaServer != nil {
		serverURL, err = app.MediaServer.Start(mediaDir)
		if err != nil {
			log.Printf("启动媒体服务器失败: %v\n", err)
			dialog.ShowError(err, app.Window)
			progress.Hide()
			return
		}
	} else {
		// 如果没有媒体服务器，使用本地文件路径（这可能只在某些设备上工作）
		serverURL = "file://" + mediaDir
	}

	// 构建媒体文件的完整URL
	mediaURL := serverURL + "/" + fileName
	// 如果选择了字幕轨道，添加字幕参数
	if app.SelectedSubtitleIndex >= 0 {
		mediaURL += "?subtitle=" + strconv.Itoa(app.SelectedSubtitleIndex)
		// 如果同时选择了音频轨道，添加音频参数
		if app.SelectedAudioIndex >= 0 {
			// 注意：这里使用的是音频轨道在音频流中的相对索引（从0开始），而不是原始索引
			// 在FFmpeg中，-map "0:a:0" 表示第一个音频流的第一个轨道
			mediaURL += "&audio=" + strconv.Itoa(0)
		}
	} else if app.SelectedAudioIndex >= 0 {
		// 只有音频轨道参数
		// 同样使用相对索引
		mediaURL += "?audio=" + strconv.Itoa(0)
	}
	log.Printf("媒体文件URL: %s\n", mediaURL)

	// 播放媒体
	err = controller.PlayMedia(mediaURL)
	if err != nil {
		log.Printf("投屏失败: %v\n", err)
		dialog.ShowError(err, app.Window)
	} else {
		log.Printf("投屏成功: %s\n", filepath.Base(app.MediaFile))
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
	progress := dialog.NewProgress("正在获取音频信息", "请稍候...", app.Window)
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
		app.AudioTracks = []AudioTrack{}
		for _, track := range audioTracks {
			app.AudioTracks = append(app.AudioTracks, AudioTrack{
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

		// 创建符合苹果设计规范的对话框布局
		dialogContent := container.NewVBox(
			container.NewPadded(descriptionLabel),
			widget.NewSeparator(), // 分隔线增强视觉层次
			container.NewPadded(paddedList),
		)

		// 创建带有取消按钮的自定义对话框，符合苹果UI设计标准
		audioDialog := dialog.NewCustomConfirm("选择音频轨道", "确定", "取消", dialogContent, func(confirmed bool) {}, app.Window)
		// 调整对话框大小以符合苹果设计风格
		audioDialog.Resize(fyne.NewSize(500, 400))

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
	progress := dialog.NewProgress("处理中...", "正在提取视频中的字幕信息", app.Window)
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
		app.SubtitleTracks = []SubtitleTrack{}
		for _, track := range subtitleTracks {
			app.SubtitleTracks = append(app.SubtitleTracks, SubtitleTrack{
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
		listContainer := container.NewVBox(paddedList)

		// 创建字幕选择对话框，符合苹果UI设计标准
		subtitleDialog := dialog.NewCustom(
			"选择字幕轨道",
			"确定",
			listContainer,
			app.Window,
		)
		// 调整对话框大小以符合苹果设计风格
		subtitleDialog.Resize(fyne.NewSize(500, 400))

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