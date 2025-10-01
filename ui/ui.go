package ui

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"GoCastify/app"
	"GoCastify/discovery"
	"GoCastify/transcoder"
	"GoCastify/types"
)

// 常量定义
const (
	progressDialogWidth  = 400
	progressDialogHeight = 200
)

// createCustomProgressDialog 创建自定义进度对话框
func createCustomProgressDialog(title, message string, parent fyne.Window) dialog.Dialog {
	// 创建标题和消息标签
	titleLabel := widget.NewLabel(title)
	titleLabel.Alignment = fyne.TextAlignCenter
	messageLabel := widget.NewLabel(message)
	messageLabel.Alignment = fyne.TextAlignCenter

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

// BuildUI 构建应用程序的用户界面 - 按照苹果Human Interface Guidelines设计
func BuildUI(app *app.App) fyne.CanvasObject {
	// 不需要自定义UI更新通道，使用Fyne的内置机制确保UI更新在主线程中执行


	// 创建FFmpeg状态提示标签 - 清晰的状态显示
	ffmpegStatusLabel := widget.NewLabel("FFmpeg: 未安装 (部分功能受限)")
	ffmpegStatusLabel.Alignment = fyne.TextAlignCenter
	ffmpegStatusLabel.Wrapping = fyne.TextWrapOff // 禁用自动换行，确保文本在一行显示
	ffmpegStatusLabel.TextStyle = fyne.TextStyle{Monospace: false}
	ffmpegStatusLabel.Resize(fyne.NewSize(400, 30)) // 设置足够的宽度，确保文本横向显示

	if app.FFmpegAvailable {
		ffmpegStatusLabel.SetText("FFmpeg: 已安装 (支持完整功能)")
	}

	// 创建居中容器以居中显示FFmpeg状态标签
	ffmpegStatusContainer := container.NewCenter(ffmpegStatusLabel)

	// 创建设备数量标签
	deviceCountLabel := widget.NewLabel("找到 0 个设备")
	deviceCountLabel.TextStyle = fyne.TextStyle{Monospace: false}
	deviceCountLabel.Alignment = fyne.TextAlignLeading

	// 创建设备列表 - 改进列表项样式以符合苹果设计
	app.DeviceList = widget.NewList(
		func() int {
			return len(app.Devices)
		},
		func() fyne.CanvasObject {
			// 使用容器来创建更好的列表项布局
			item := widget.NewLabel("设备名称")
			item.Wrapping = fyne.TextTruncate
			item.Alignment = fyne.TextAlignLeading
			return container.NewMax(item)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= 0 && id < len(app.Devices) {
				container := obj.(*fyne.Container)
				label := container.Objects[0].(*widget.Label)
				label.SetText(getFriendlyDeviceName(app.Devices[id]))
				// 为选中项添加视觉反馈
				if id == app.SelectedDeviceIndex {
					label.TextStyle = fyne.TextStyle{Bold: true}
				} else {
					label.TextStyle = fyne.TextStyle{}
				}
			}
		},
	)

	// 创建设备列表选中事件 - 添加视觉反馈
	app.DeviceList.OnSelected = func(id widget.ListItemID) {
		app.SelectedDeviceIndex = id
		app.DeviceList.Refresh() // 刷新列表以显示选中状态
	}

	// 创建搜索设备按钮 - 使用苹果风格的操作按钮
	searchButton := widget.NewButton("搜索设备", func() {
		// 如果已经有搜索上下文在运行，取消它
		if app.SearchCancel != nil {
			app.SearchCancel()
		}

		// 创建新的上下文用于取消操作
		ctx, cancel := app.CreateSearchContext()
		app.SearchCancel = cancel

		// 显示进度对话框
		progressMessage := "正在搜索DLNA设备..."
		progress := createCustomProgressDialog("搜索中...", progressMessage, app.Window)
		progress.Show()

		// 更新状态标签
		ffmpegStatusLabel.SetText("正在搜索DLNA设备...")

		// 创建设备发现器实例
		discoverer := discovery.NewSSDPDiscoverer()

		// 清空当前设备列表
		app.Devices = []types.DeviceInfo{}
		app.DeviceList.Refresh()

		// 启动goroutine搜索设备
		go func() {
			// 使用回调函数处理发现的设备
			onDeviceFound := func(device types.DeviceInfo) {
				// 在主线程中更新UI
				time.AfterFunc(0, func() {
					// 添加设备到列表
					app.Devices = append(app.Devices, device)
					app.DeviceList.Refresh()
					// 更新设备数量标签
					deviceCountLabel.SetText(fmt.Sprintf("找到 %d 个设备", len(app.Devices)))
				})
			}

			// 开始搜索设备
			err := discoverer.StartSearchWithContext(ctx, onDeviceFound)
			if err != nil {
				log.Printf("搜索设备失败: %v\n", err)
			}

			// 在主线程中更新设备数量标签
			time.AfterFunc(0, func() {
				deviceCountLabel.SetText(fmt.Sprintf("找到 %d 个设备", len(app.Devices)))
				app.Window.Canvas().Refresh(deviceCountLabel)
			})
			
			// 使用time.AfterFunc确保UI更新在主线程中执行
			time.AfterFunc(0, func() {
				// 隐藏进度对话框
				progress.Hide()

				// 恢复FFmpeg状态显示
				if app.FFmpegAvailable {
					ffmpegStatusLabel.SetText("FFmpeg: 已安装 (支持完整功能)")
				} else {
					ffmpegStatusLabel.SetText("FFmpeg: 未安装 (部分功能受限)")
				}

				// 如果没有找到设备，显示提示
				if len(app.Devices) == 0 {
					dialog.ShowInformation("未找到设备", "未找到任何DLNA设备。\n请确保您的设备已开启并连接到同一网络。", app.Window)
				}

				// 刷新设备列表和窗口内容
				app.DeviceList.Refresh()
				app.Window.Canvas().Refresh(app.Window.Content())

				// 清理
				app.SearchCancel = nil
			})
		}()
	})

	// 创建媒体文件标签和选择按钮 - 改进标签样式
	mediaFileLabel := widget.NewLabel("未选择文件")
	mediaFileLabel.Wrapping = fyne.TextWrapWord
	mediaFileLabel.TextStyle = fyne.TextStyle{Monospace: false}

	// 创建音频相关的UI组件（需要在selectFileButton之前定义，因为它会被使用）
audioLabel := widget.NewLabel("音轨: 默认")
audioLabel.Wrapping = fyne.TextWrapWord
audioLabel.TextStyle = fyne.TextStyle{Monospace: false}
audioSelectButton := widget.NewButton("选择音轨", func() {
		app.SelectAudio(audioLabel)
	})

	selectFileButton := widget.NewButton("选择文件", func() {
		// 使用文件选择对话框并设置合适的大小
		fileCallback := func(file fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, app.Window)
				return
			}

			if file != nil {
				defer file.Close()
				app.MediaFile = file.URI().Path()
				mediaFileLabel.SetText(filepath.Base(app.MediaFile))
				app.SelectedAudioIndex = -1
				audioLabel.SetText("音轨: 默认")

				supported, needTranscode := transcoder.IsSupportedFormat(app.MediaFile)
				if !supported {
					dialog.ShowInformation("不支持的格式", "当前文件格式不受支持，请选择其他文件。", app.Window)
					return
				}

				if needTranscode && !transcoder.CheckFFmpeg() {
					dialog.ShowInformation("转码功能不可用", "文件需要转码，但未找到FFmpeg。\n请安装FFmpeg以支持非MP4格式的视频。", app.Window)
				}
			}
		}

		// 创建文件对话框并设置更大的尺寸
		obtainer := dialog.NewFileOpen(fileCallback, app.Window)
		obtainer.Resize(fyne.NewSize(800, 600)) // 设置更大的窗口尺寸
		obtainer.Show()
	})

	// 投屏按钮 - 作为主要操作按钮，使用更突出的布局
	castButton := widget.NewButton("开始投屏", func() {
		// 检查是否选择了设备
		if app.SelectedDeviceIndex < 0 || app.SelectedDeviceIndex >= len(app.Devices) {
			dialog.ShowInformation("提示", "请先选择要投屏的设备", app.Window)
			return
		}

		// 检查是否选择了文件
		if app.MediaFile == "" {
			dialog.ShowInformation("提示", "请先选择要投屏的文件", app.Window)
			return
		}

		// 检查文件格式是否支持
		supported, needTranscode := transcoder.IsSupportedFormat(app.MediaFile)
		if !supported {
			dialog.ShowInformation("不支持的格式", "当前文件格式不受支持，请选择其他文件。", app.Window)
			return
		}

		// 如果需要转码，检查FFmpeg是否可用
		if needTranscode || (app.SelectedAudioIndex >= 0) {
			if !transcoder.CheckFFmpeg() {
				dialog.ShowInformation("转码功能不可用", "文件需要转码或选择音轨，但未找到FFmpeg。\n请安装FFmpeg以支持这些功能。", app.Window)
				return
			}
		}

		// 显示加载对话框
		progressMessage := "正在准备媒体文件并连接设备..."
		progressDialog := createCustomProgressDialog("投屏中...", progressMessage, app.Window)
		progressDialog.Show()

		// 在后台执行投屏
		go func() {
			// 创建带超时的上下文
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			
			err := app.StartCastingWithContext(ctx, progressDialog)
			if err != nil {
				log.Printf("投屏操作失败: %v\n", err)
				dialog.ShowError(err, app.Window)
			} else {
				dialog.ShowInformation("成功", "投屏成功！\n媒体文件正在通过HTTP服务器提供", app.Window)
			}
			
			// 关闭加载对话框
			progressDialog.Hide()
		}()
	})

	// 使用提示 - 改进文本样式和排版
	tipsText := "1. 点击'搜索设备'查找局域网中的DLNA设备\n"
	tipsText += "2. 从列表中选择要投屏的设备\n"
	tipsText += "3. 点击'选择文件'选择要投屏的视频文件\n"
	tipsText += "4. 点击'开始投屏'开始媒体播放\n\n"
	tipsText += "注意：\n"
	tipsText += "- MP4格式通常无需转码即可直接播放\n"
	tipsText += "- 其他格式可能需要安装FFmpeg进行转码\n"
	tipsText += "- 支持选择视频中的音轨"

	tipsLabel := widget.NewLabel(tipsText)
	tipsLabel.Wrapping = fyne.TextWrapWord
	tipsLabel.TextStyle = fyne.TextStyle{Monospace: false}

	// 创建主布局 - 改进整体布局，增加更好的分组和间距（符合苹果HIG）
	topLayout := container.NewCenter(
		container.NewPadded(
			searchButton,
		),
	)

	// 使用自定义卡片效果包装设备列表 - 改进卡片样式
	deviceCard := createCard(
		"可用设备",
		deviceCountLabel,
		app.DeviceList,
	)
	// 设置卡片最小高度
	size := deviceCard.MinSize()
	if size.Height < 200 {
		size.Height = 200
	}
	deviceCard.Resize(size)

	// 创建使用指南描述标签
	tipsDescLabel := widget.NewLabel("简单四步，轻松投屏")
	tipsDescLabel.TextStyle = fyne.TextStyle{Italic: false}
	tipsDescLabel.Alignment = fyne.TextAlignLeading
	
	// 使用自定义卡片效果包装使用提示
	tipsCard := createCard(
		"使用指南",
		tipsDescLabel,
		tipsLabel,
	)
	// 设置卡片最小高度
	size = tipsCard.MinSize()
	if size.Height < 200 {
		size.Height = 200
	}
	tipsCard.Resize(size)

	// 创建文件选择卡片
	fileSelectContent := container.NewVBox(
		container.NewPadded(mediaFileLabel),
		container.NewPadded(audioLabel),
		container.NewHBox(
			layout.NewSpacer(),
			selectFileButton,
			audioSelectButton,
			layout.NewSpacer(),
		),
	)
	// 创建文件选择描述标签
	fileDescLabel := widget.NewLabel("请选择要投屏的视频文件")
	fileDescLabel.TextStyle = fyne.TextStyle{Italic: false}
	fileDescLabel.Alignment = fyne.TextAlignLeading
	
	fileCard := createCard(
		"选择文件",
		fileDescLabel,
		fileSelectContent,
	)

	// 底部布局 - 突出主要操作
	bottomLayout := container.NewVBox(
		fileCard,
		layout.NewSpacer(), // 增加间距
		fyne.NewContainerWithLayout(layout.NewCenterLayout(),
			container.NewPadded(
				castButton,
			),
		),
	)

	// 主内容布局 - 符合苹果HIG的间距和分组
	content := container.NewPadded(
		container.NewVBox(
			fyne.NewContainerWithLayout(layout.NewCenterLayout(), ffmpegStatusContainer),
			layout.NewSpacer(), // 增加间距
			widget.NewSeparator(),
			layout.NewSpacer(), // 增加间距
			fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(2),
				deviceCard,
				tipsCard,
			),
			layout.NewSpacer(), // 增加间距
			topLayout,
			layout.NewSpacer(), // 增加间距
			widget.NewSeparator(),
			layout.NewSpacer(), // 增加间距
			bottomLayout,
		),
	)

	return content
}

// createCard 创建一个符合苹果设计风格的带标题和描述的卡片
func createCard(title string, descriptionLabel *widget.Label, content fyne.CanvasObject) fyne.CanvasObject {
	titleLabel := widget.NewLabel(title)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true} // 标题使用粗体
	titleLabel.Alignment = fyne.TextAlignLeading
	titleLabel.Resize(fyne.NewSize(400, 25))

	descLabel := descriptionLabel
	descLabel.Resize(fyne.NewSize(400, 20))

	// 创建带内边距的内容容器，增加留白空间
	paddedContent := container.NewPadded(content)

	cardContent := container.NewVBox(
		container.NewPadded(titleLabel),  // 添加内边距
		container.NewPadded(descLabel),   // 添加内边距
		widget.NewSeparator(),
		paddedContent,
		layout.NewSpacer(), // 增加内容的间距
	)

	// 在Fyne v2中使用容器嵌套来创建卡片效果 - 更符合苹果设计的卡片样式
	card := container.NewPadded(
		fyne.NewContainerWithLayout(
			&borderLayout{},
			widget.NewSeparator(),
			widget.NewSeparator(),
			widget.NewSeparator(),
			widget.NewSeparator(),
			cardContent,
		),
	)

	return card
}

// getFriendlyDeviceName 获取设备的友好名称
func getFriendlyDeviceName(device types.DeviceInfo) string {
	if device.FriendlyName != "" {
		return device.FriendlyName
	}
	// 从Location URL提取设备信息
	parts := strings.Split(device.Location, "/")
	if len(parts) > 2 {
		return parts[2] // 返回主机名或IP
	}
	return "未知设备"
}

// borderLayout 简单的边框布局
// 用于实现卡片的边框效果
type borderLayout struct{}

// MinSize 计算布局的最小尺寸
func (b *borderLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) < 5 {
		return fyne.NewSize(0, 0)
	}
	inner := objects[4].MinSize()
	return fyne.NewSize(inner.Width+4, inner.Height+4)
}

// Layout 布局子组件
func (b *borderLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 5 {
		return
	}
	tl := objects[0]      // 顶部边框
	rb := objects[1]      // 右侧边框
	br := objects[2]      // 底部边框
	lb := objects[3]      // 左侧边框
	content := objects[4] // 内容

	tl.Resize(fyne.NewSize(size.Width, 1))
	tl.Move(fyne.NewPos(0, 0))

	rb.Resize(fyne.NewSize(1, size.Height))
	rb.Move(fyne.NewPos(size.Width-1, 0))

	br.Resize(fyne.NewSize(size.Width, 1))
	br.Move(fyne.NewPos(0, size.Height-1))

	lb.Resize(fyne.NewSize(1, size.Height))
	lb.Move(fyne.NewPos(0, 0))

	content.Resize(fyne.NewSize(size.Width-2, size.Height-2))
	content.Move(fyne.NewPos(1, 1))
}

// videoFileFilter 实现dialog.FileFilter接口，用于过滤视频文件
type videoFileFilter struct{}

// Name 返回过滤器的显示名称
func (f *videoFileFilter) Name() string {
	return "视频文件 (*.mp4, *.mkv, *.avi, *.wmv, *.flv, *.mov, *.mpg, *.mpeg, *.webm)"
}

// Matches 判断一个URI是否符合过滤条件
func (f *videoFileFilter) Matches(uri fyne.URI) bool {
	if uri == nil {
		return false
	}
	// 直接使用URI的Scheme和Path来进行判断
	if uri.Scheme() != "file" {
		return false
	}
	path := uri.Path()
	ext := strings.ToLower(filepath.Ext(path))
	supportedExts := []string{"mp4", "mkv", "avi", "wmv", "flv", "mov", "mpg", "mpeg", "webm"}
	for _, supportedExt := range supportedExts {
		if ext == "."+supportedExt {
			return true
		}
	}
	return false
}
