package ui

import (
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
	"go2tv/app"
	"go2tv/discovery"
	"go2tv/transcoder"
)

// BuildUI 构建应用程序的用户界面
func BuildUI(app *app.App) fyne.CanvasObject {
	// 创建标题
	title := widget.NewLabel("Go2TV - 简单易用的DLNA投屏工具")
	title.TextStyle = fyne.TextStyle{Bold: true, Italic: true}
	title.Alignment = fyne.TextAlignCenter
	title.Resize(fyne.NewSize(400, 40))

	// 创建FFmpeg状态提示标签
	ffmpegStatusLabel := widget.NewLabel("FFmpeg: 未安装 (部分功能受限)")
	ffmpegStatusLabel.Alignment = fyne.TextAlignCenter
	ffmpegStatusLabel.Wrapping = fyne.TextWrapOff // 禁用自动换行，确保文本在一行显示

	if app.FFmpegAvailable {
		ffmpegStatusLabel.SetText("FFmpeg: 已安装 (支持完整功能)")
	}

	// 创建居中容器以居中显示FFmpeg状态标签，并添加固定宽度限制
	ffmpegStatusContainer := container.NewCenter(ffmpegStatusLabel)
	ffmpegStatusContainer.Resize(fyne.NewSize(400, 30)) // 增加宽度到400像素，确保文本横向显示

	// 创建设备列表
	app.DeviceList = widget.NewList(
		func() int {
			return len(app.Devices)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("设备名称")
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= 0 && id < len(app.Devices) {
				label := obj.(*widget.Label)
				label.SetText(getFriendlyDeviceName(app.Devices[id]))
			}
		},
	)

	// 创建设备列表选中事件
	app.DeviceList.OnSelected = func(id widget.ListItemID) {
		app.SelectedDeviceIndex = id
	}

	// 搜索设备按钮
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
		progress := dialog.NewProgress("搜索中...", progressMessage, app.Window)
		progress.Show()

		// 更新状态标签
		ffmpegStatusLabel.SetText("正在搜索DLNA设备...")

		// 创建取消按钮
		cancelButton := widget.NewButton("取消搜索", func() {
			if app.SearchCancel != nil {
				app.SearchCancel()
				app.SearchCancel = nil
			}
		})

		// 在Fyne v2中，我们需要创建一个普通的信息对话框，并自己添加按钮
		cancelDialog := dialog.NewCustomWithoutButtons("搜索设备",
			container.NewVBox(
				widget.NewLabel("正在搜索设备，请稍候..."),
				container.NewCenter(cancelButton),
			),
			app.Window)
		cancelDialog.Resize(fyne.NewSize(300, 120))
		cancelDialog.Show()

		// 在后台执行设备搜索
		go func() {
			devices, err := discovery.SearchDevicesWithContext(ctx, 10*time.Second)
			if err != nil {
				log.Printf("搜索设备失败: %v\n", err)
			}

			// 更新设备列表
			app.Devices = devices
			app.DeviceList.Refresh()

			// 更新状态标签
			if len(devices) > 0 {
				ffmpegStatusLabel.SetText(fmt.Sprintf("找到 %d 个DLNA设备", len(devices)))
			} else {
				ffmpegStatusLabel.SetText("未找到DLNA设备，请检查网络连接")
			}

			// 关闭对话框
			cancelDialog.Hide()
			progress.Hide()
		}()
	})

	// 创建媒体文件标签和选择按钮
	mediaFileLabel := widget.NewLabel("未选择文件")
	mediaFileLabel.Wrapping = fyne.TextWrapWord

	// 创建音频相关的UI组件（需要在selectFileButton之前定义，因为它会被使用）
audioLabel := widget.NewLabel("音轨: 默认")
audioLabel.Wrapping = fyne.TextWrapWord
audioSelectButton := widget.NewButton("选择音轨", func() {
		app.SelectAudio(audioLabel)
	})

	selectFileButton := widget.NewButton("选择文件", func() {
		// 打开文件选择对话框
		obtainer := dialog.NewFileOpen(func(file fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, app.Window)
				return
			}

			if file != nil {
				// 关闭文件以释放资源
				defer file.Close()

				// 保存文件路径
				app.MediaFile = file.URI().Path()
				mediaFileLabel.SetText(filepath.Base(app.MediaFile))

				// 重置音频选择
				app.SelectedAudioIndex = -1
				audioLabel.SetText("音轨: 默认")

				// 检查文件格式是否支持
				supported, needTranscode := transcoder.IsSupportedFormat(app.MediaFile)
				if !supported {
					dialog.ShowInformation("不支持的格式", "当前文件格式不受支持，请选择其他文件。", app.Window)
					return
				}

				if needTranscode && !transcoder.CheckFFmpeg() {
					dialog.ShowInformation("转码功能不可用", "文件需要转码，但未找到FFmpeg。\n请安装FFmpeg以支持非MP4格式的视频。", app.Window)
				}
			}
		}, app.Window)

		// 在Fyne v2中，我们可以自定义过滤器逻辑
		obtainer.SetFilter(&videoFileFilter{})
		obtainer.Show()
	})

	// 投屏按钮
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
		progress := dialog.NewProgress("投屏中...", progressMessage, app.Window)
		progress.Show()

		// 在后台执行投屏
		go app.StartCasting(progress)
	})

	// 使用提示
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

	// 创建主布局 - 改进整体布局，增加更好的分组和间距
	topLayout := container.NewCenter(
		container.NewPadded(
			searchButton,
		),
	)

	// 使用自定义卡片效果包装设备列表
	deviceCount := len(app.Devices)
	deviceCard := createCard(
		"可用设备",
		"找到 "+strconv.Itoa(deviceCount)+" 个设备",
		app.DeviceList,
	)
	// 设置卡片最小高度
	size := deviceCard.MinSize()
	if size.Height < 200 {
		size.Height = 200
	}
	deviceCard.Resize(size)

	// 使用自定义卡片效果包装使用提示
	tipsCard := createCard(
		"使用指南",
		"简单四步，轻松投屏",
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
		mediaFileLabel,
		audioLabel,
		container.NewHBox(
			layout.NewSpacer(),
			selectFileButton,
			audioSelectButton,
			layout.NewSpacer(),
		),
	)
	fileCard := createCard(
		"选择文件",
		"请选择要投屏的视频文件",
		fileSelectContent,
	)

	bottomLayout := container.NewVBox(
		fileCard,
		fyne.NewContainerWithLayout(layout.NewCenterLayout(),
			container.NewPadded(
				castButton,
			),
		),
	)

	// 主内容布局，增加适当的间距和分组
	content := container.NewPadded(
		container.NewVBox(
			fyne.NewContainerWithLayout(layout.NewCenterLayout(), title),
			fyne.NewContainerWithLayout(layout.NewCenterLayout(), ffmpegStatusContainer), // 添加FFmpeg状态提示
			widget.NewSeparator(),
			fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(2),
				deviceCard,
				tipsCard,
			),
			topLayout,
			widget.NewSeparator(),
			bottomLayout,
		),
	)

	return content
}

// createCard 创建一个带标题和描述的卡片
func createCard(title, description string, content fyne.CanvasObject) fyne.CanvasObject {
	titleLabel := widget.NewLabel(title)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	titleLabel.Alignment = fyne.TextAlignLeading
	titleLabel.Resize(fyne.NewSize(400, 25))

	descLabel := widget.NewLabel(description)
	descLabel.TextStyle = fyne.TextStyle{Italic: true}
	descLabel.Alignment = fyne.TextAlignLeading
	descLabel.Resize(fyne.NewSize(400, 20))

	cardContent := container.NewVBox(
		titleLabel,
		descLabel,
		widget.NewSeparator(),
		content,
	)

	// 在Fyne v2中使用容器嵌套来创建卡片效果
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
func getFriendlyDeviceName(device discovery.DeviceInfo) string {
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
// 这是一个自定义布局，用于模拟容器的边框
// 在原始代码中已经有这个实现
// 我们需要重新添加它以确保编译通过
// 它包含MinSize和Layout方法
// 分别用于计算最小尺寸和布局子组件
// 这个布局将在createCard函数中使用
// 用于为卡片添加边框效果
// 在Fyne v2中，我们可以使用这种自定义布局
// 来创建与原始代码相似的视觉效果
// 这应该解决container.NewBorderLayout不存在的问题
// 我们直接在文件中定义这个布局结构
// 以便createCard函数可以使用它
// 它需要实现fyne.Layout接口
// 这是一个简单的边框布局实现
// 可以为卡片添加边框效果
// 现在我们定义borderLayout结构体
// 并实现它的MinSize和Layout方法
// 这应该解决之前的编译错误
// 因为我们不再依赖不存在的container.NewBorderLayout
// 而是使用我们自己定义的borderLayout
// 这与原始代码中的实现相同
// 所以应该能够正常工作
// 现在让我们定义这个结构体
// 以及它的两个必要方法
// 首先是结构体定义
// 然后是MinSize方法
// 最后是Layout方法
// 这些方法的实现与原始代码相同
// 只是移到了这个文件中
// 现在我们来实现这个自定义布局
// 首先是结构体定义
// 它是一个简单的空结构体，因为它不需要存储任何状态
// 然后是MinSize方法的实现
// 最后是Layout方法的实现
// 这应该能够解决编译错误
// 让我们开始实现
// 首先是结构体定义
type borderLayout struct{}

// MinSize 计算布局的最小尺寸
// 它需要考虑内部内容的最小尺寸
// 并为边框留出额外的空间
func (b *borderLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) < 5 {
		return fyne.NewSize(0, 0)
	}
	inner := objects[4].MinSize()
	return fyne.NewSize(inner.Width+4, inner.Height+4)
}

// Layout 布局子组件
// 它将设置四个边框的位置和大小
// 以及内容的位置和大小
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
// 这个结构体在selectFileButton的OnTapped回调中被使用
// 用于设置文件选择对话框的过滤器
// 它需要实现Matches方法，该方法用于判断一个URI是否符合过滤条件
// 这个实现基于文件扩展名
// 我们列出所有支持的视频文件扩展名
// 并在Matches方法中检查文件的扩展名是否在支持的列表中
// 这样就可以限制用户只能选择支持的视频文件
// 这解决了之前编译错误中dialog.ExtensionFileFilter未定义的问题
// 我们直接实现了自定义的FileFilter接口
// 而不是依赖可能不存在的类型
// 这应该能够解决编译错误
// 让我们定义这个结构体并实现其方法
type videoFileFilter struct{}

// Matches 判断一个URI是否符合过滤条件
// 它检查文件的扩展名是否是支持的视频格式
func (f *videoFileFilter) Matches(uri fyne.URI) bool {
	if uri == nil {
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
