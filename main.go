package main

import (
	"context"
	"image/color"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"go2tv/discovery"
	"go2tv/dlna"
	"go2tv/server"
)

// 定义主应用程序结构体
type Go2TVApp struct {
	window         fyne.Window
	app            fyne.App
	devices        []discovery.DeviceInfo
	deviceList     *widget.List
	selectedDevice *discovery.DeviceInfo
	mediaFile      string
	mediaServer    *server.MediaServer
}

// getFriendlyDeviceName 从原始设备名称中提取更友好的显示名称
func getFriendlyDeviceName(originalName string) string {
	// 如果名称包含 "KDL-"，这通常是Sony电视型号
	if strings.Contains(originalName, "KDL-") {
		// 提取 KDL- 后的型号部分
		parts := strings.Split(originalName, "KDL-")
		if len(parts) > 1 {
			// 取 KDL- 后的部分并截断到第一个非字母数字字符
			modelPart := parts[1]
			for i := 0; i < len(modelPart); i++ {
				if !strings.ContainsAny(string(modelPart[i]), "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-") {
					modelPart = modelPart[:i]
					break
				}
			}
			return "Sony TV (KDL-" + modelPart + ")"
		}
	}
	
	// 检查是否包含常见电视品牌关键词
	brandKeywords := map[string]string{
		"LG": "LG TV",
		"Samsung": "Samsung TV",
		"Panasonic": "Panasonic TV",
		"Sharp": "Sharp TV",
		"Toshiba": "Toshiba TV",
		"Philips": "Philips TV",
		"Hisense": "Hisense TV",
		"TCL": "TCL TV",
	}
	
	for keyword, displayName := range brandKeywords {
		if strings.Contains(strings.ToLower(originalName), strings.ToLower(keyword)) {
			return displayName
		}
	}
	
	// 如果包含 UPnP/ 或类似的服务器信息，提取前面的部分
	if idx := strings.Index(originalName, " UPnP/"); idx > 0 {
		return originalName[:idx]
	}
	
	// 如果包含 / 分隔符，提取第一部分
	if idx := strings.Index(originalName, "/"); idx > 0 {
		firstPart := originalName[:idx]
		// 如果第一部分不是太短（至少3个字符）且不是数字，就用它
		if len(firstPart) >= 3 && !strings.ContainsAny(firstPart, "0123456789") {
			return firstPart
		}
	}
	
	// 默认返回原始名称，但限制长度
	if len(originalName) > 30 {
		return originalName[:27] + "..."
	}
	return originalName
}

func main() {
	// 创建Fyne应用，使用唯一ID以避免Preferences API错误
	app := app.NewWithID("com.example.go2tv")
	window := app.NewWindow("Go2TV - 无线投屏")

	// 设置窗口大小为更大的尺寸，提供更好的用户体验
	window.Resize(fyne.NewSize(800, 600))
	// 设置最小窗口尺寸
	window.SetFixedSize(false)

	// 创建应用实例
	go2tvApp := &Go2TVApp{
		window:      window,
		app:         app,
		mediaServer: server.NewMediaServer(8080), // 使用8080端口启动媒体服务器
	}

	// 设置主题颜色
	app.Settings().SetTheme(&customTheme{})

	// 构建UI
	go2tvApp.buildUI()

	// 显示窗口并运行应用
	window.ShowAndRun()
}

// 自定义主题 - 简化版本以确保兼容性
type customTheme struct{}

// 定义主题颜色
func (m *customTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	// 使用字符串比较来确保兼容性
	if name == "primary" {
		return color.RGBA{R: 0x33, G: 0x99, B: 0xff, A: 0xff} // 蓝色主题
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (m *customTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (m *customTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (m *customTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}

// 自定义卡片组件 - 替代container.NewCard
func createCard(title, subtitle string, content fyne.CanvasObject) fyne.CanvasObject {
	cardTitle := widget.NewLabel(title)
	cardTitle.TextStyle = fyne.TextStyle{Bold: true}
	
	cardSubtitle := widget.NewLabel(subtitle)
	cardSubtitle.TextStyle = fyne.TextStyle{Italic: true}
	// 在Fyne 2.0中不直接支持TextSize，通过文本样式调整
	
	head := container.NewVBox(cardTitle, cardSubtitle)
	card := container.NewBorder(
		head,
		nil,
		nil,
		nil,
		container.NewPadded(content),
	)
	
	// 添加边框效果
	card = container.NewPadded(
		fyne.NewContainerWithLayout(
			&borderLayout{},
			widget.NewSeparator(),
			widget.NewSeparator(),
			widget.NewSeparator(),
			widget.NewSeparator(),
			card,
		),
	)
	
	return card
}

// 简单的边框布局
// 用于实现卡片的边框效果
type borderLayout struct{}

func (b *borderLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	inner := objects[4].MinSize()
	return fyne.NewSize(inner.Width+4, inner.Height+4)
}

func (b *borderLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	tl := objects[0] // 顶部边框
	rb := objects[1] // 右侧边框
	br := objects[2] // 底部边框
	lb := objects[3] // 左侧边框
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

// 构建用户界面
func (app *Go2TVApp) buildUI() {
	// 标题 - 更大更醒目的标题
	title := widget.NewLabel("Go2TV - 无线投屏")
	title.TextStyle = fyne.TextStyle{Bold: true, Italic: false}
	title.Alignment = fyne.TextAlignCenter
	// Fyne 2.0中不直接支持TextSize，通过修改主题来调整

	// 设备列表 - 使用更美观的列表样式
	// 使用最简单的设备列表实现
	app.deviceList = widget.NewList(
		func() int {
			return len(app.devices)
		},
		func() fyne.CanvasObject {
			// 创建最简单的列表项，只包含两个标签，不使用复杂布局
			return container.NewVBox(
				widget.NewLabel("设备名称"),
				widget.NewLabel("设备地址"),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			// 添加调试日志
			log.Printf("更新列表项 %d，设备总数: %d\n", id, len(app.devices))
			
			// 检查索引是否有效
			if id < 0 || id >= len(app.devices) {
				log.Printf("警告：无效的设备索引 %d\n", id)
				return
			}
			
			// 获取VBox容器
			vbox, ok := obj.(*fyne.Container)
			if !ok || vbox.Layout == nil {
				log.Printf("错误：无法将obj转换为有效的fyne.Container\n")
				return
			}
			
			// 检查容器中的对象数量
			if len(vbox.Objects) < 2 {
				log.Printf("错误：容器中对象数量不足2个\n")
				return
			}
			
			// 获取并更新名称标签
			nameLabel, ok := vbox.Objects[0].(*widget.Label)
			if ok {
				// 获取原始设备名称
				originalName := app.devices[id].FriendlyName
				// 优化设备名称显示，提取更友好的名称
				friendlyName := getFriendlyDeviceName(originalName)
				nameLabel.SetText(friendlyName)
				nameLabel.TextStyle = fyne.TextStyle{Bold: true}
				nameLabel.Refresh()
			} else {
				log.Printf("错误：无法将第一个对象转换为widget.Label\n")
			}
			
			// 获取并更新地址标签
			locationLabel, ok := vbox.Objects[1].(*widget.Label)
			if ok {
				locationLabel.SetText(app.devices[id].Location)
				locationLabel.TextStyle = fyne.TextStyle{Italic: true}
				locationLabel.Refresh()
			} else {
				log.Printf("错误：无法将第二个对象转换为widget.Label\n")
			}
			
			// 刷新整个容器
			vbox.Refresh()
		},
	)

	// 用于跟踪选中的设备索引
	var selectedIndex int
	var lastSelectedIndex = -1
	app.deviceList.OnSelected = func(id widget.ListItemID) {
		lastSelectedIndex = selectedIndex
		selectedIndex = id
		app.deviceList.RefreshItem(lastSelectedIndex) // 刷新上一个选中项
		app.deviceList.RefreshItem(selectedIndex)     // 刷新当前选中项
	}
	app.deviceList.OnUnselected = func(id widget.ListItemID) {
		app.deviceList.RefreshItem(id) // 刷新取消选中的项
	}

	// 搜索设备按钮
	searchButton := widget.NewButton("搜索设备", func() {
		// 创建取消上下文，用于取消搜索
		cancelCtx, cancel := context.WithCancel(context.Background())
		isCancelled := uint32(0)
		
		// 创建进度条
		progressBar := widget.NewProgressBar()
		progressBar.Min = 0
		progressBar.Max = 1.0
		progressBar.SetValue(0.0)
		
		// 创建状态标签
		statusLabel := widget.NewLabel("正在搜索局域网中的投屏设备...")
		
		// 创建停止按钮
		stopButton := widget.NewButton("停止", func() {
			// 设置取消标志
			atomic.StoreUint32(&isCancelled, 1)
			// 取消搜索
			cancel()
			statusLabel.SetText("搜索已停止")
		})
		
		// 创建对话框内容
		content := container.NewVBox(
			statusLabel,
			progressBar,
			layout.NewSpacer(),
			stopButton,
		)
		
		// 创建自定义对话框
		searchDialog := dialog.NewCustom("搜索设备", "关闭", content, app.window)
		
		// 显示对话框
		searchDialog.Show()

		// 在后台搜索设备
		go func() {
			// 创建一个计时器来更新进度条
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			
			// 记录开始时间
			startTime := time.Now()
			searchTimeout := 5 * time.Second
			
			// 更新进度条的goroutine
			go func() {
				for range ticker.C {
					// 检查是否已取消
					if atomic.LoadUint32(&isCancelled) == 1 {
						return
					}
					
					// 计算已过去的时间比例
					elapsed := time.Since(startTime)
					progress := elapsed.Seconds() / searchTimeout.Seconds()
					if progress > 1.0 {
						progress = 1.0
					}
					progressBar.SetValue(progress)
				}
			}()
			
			// 执行搜索，传递取消上下文
			devices, err := discovery.SearchDevicesWithContext(cancelCtx, searchTimeout)
			
			// 检查是否已取消
			if atomic.LoadUint32(&isCancelled) == 1 {
				log.Println("搜索已被用户取消")
			} else if err != nil {
				log.Printf("搜索设备失败: %v\n", err)
				dialog.ShowError(err, app.window)
			} else {
				app.devices = devices
				app.deviceList.Refresh()
				log.Printf("发现 %d 个设备\n", len(devices))
				
				// 刷新设备数量显示
				app.window.Content().Refresh()
			}
			
			// 等待一小段时间让用户看到最终状态
			time.Sleep(500 * time.Millisecond)
			
			// 关闭对话框
			searchDialog.Hide()
		}()
	})

	// 选择文件按钮
	mediaFileLabel := widget.NewLabel("请选择要投屏的视频文件")
	mediaFileLabel.Wrapping = fyne.TextWrapWord
	mediaFileLabel.Alignment = fyne.TextAlignCenter
	selectFileButton := widget.NewButton("选择文件", func() {
		// 为了确保能够正确显示文件，我们先尝试不设置过滤器
		// 这样用户可以看到所有文件，然后手动选择视频文件
		dialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, app.window)
				return
			}

			if reader != nil {
				defer reader.Close()
				// 获取文件路径
				filePath := reader.URI().Path()
				app.mediaFile = filePath
				mediaFileLabel.SetText("已选择文件: " + filepath.Base(filePath))
				mediaFileLabel.Refresh()
			}
		}, app.window)

		// 美化文件选择对话框
		dialog.Resize(fyne.NewSize(900, 700)) // 增大对话框尺寸
		dialog.SetConfirmText("选择")
		dialog.SetDismissText("取消")
		dialog.Show()
	})

	// 投屏按钮
	castButton := widget.NewButton("开始投屏", func() {
		// 检查是否选择了设备和文件
		if len(app.devices) == 0 {
			dialog.ShowInformation("提示", "请先搜索并选择设备", app.window)
			return
		}

		// 检查是否选择了设备
		if selectedIndex < 0 {
			dialog.ShowInformation("提示", "请先选择一个设备", app.window)
			return
		}

		if app.mediaFile == "" {
			dialog.ShowInformation("提示", "请先选择要投屏的MP4文件", app.window)
			return
		}

		// 显示加载对话框
		progress := dialog.NewProgress("投屏中...", "正在连接设备并准备投屏", app.window)
		progress.Show()

		// 在后台执行投屏
		go func() {
			selectedDevice := app.devices[selectedIndex]
			log.Printf("连接设备: %s, 地址: %s\n", selectedDevice.FriendlyName, selectedDevice.Location)

			// 创建设备控制器
			controller, err := dlna.NewDeviceController(selectedDevice.Location)
			if err != nil {
				log.Printf("创建设备控制器失败: %v\n", err)
				dialog.ShowError(err, app.window)
				progress.Hide()
				return
			}

			// 获取文件所在目录
			mediaDir := filepath.Dir(app.mediaFile)
			fileName := filepath.Base(app.mediaFile)

			// 启动媒体服务器并获取媒体文件的HTTP URL
			serverURL, err := app.mediaServer.Start(mediaDir)
			if err != nil {
				log.Printf("启动媒体服务器失败: %v\n", err)
				dialog.ShowError(err, app.window)
				progress.Hide()
				return
			}

			// 构建媒体文件的完整URL
			mediaURL := serverURL + "/" + fileName
			log.Printf("媒体文件URL: %s\n", mediaURL)

			// 播放媒体
			err = controller.PlayMedia(mediaURL)
			if err != nil {
				log.Printf("投屏失败: %v\n", err)
				dialog.ShowError(err, app.window)
			} else {
				log.Printf("投屏成功: %s\n", filepath.Base(app.mediaFile))
				dialog.ShowInformation("成功", "投屏成功！\n媒体文件正在通过HTTP服务器提供", app.window)
			}

			// 关闭加载对话框
			progress.Hide()
		}()
	})

	// 添加使用提示
	tipsLabel := widget.NewLabel("使用说明:\n1. 点击\"搜索设备\"查找局域网中的投屏设备\n2. 选择要投屏的设备\n3. 点击\"选择文件\"选择视频文件\n4. 点击\"开始投屏\"开始播放")
	tipsLabel.TextStyle = fyne.TextStyle{Italic: true}
	tipsLabel.Wrapping = fyne.TextWrapWord

	// 美化搜索按钮 - 添加图标和更好的视觉效果
	searchButton.Importance = widget.HighImportance
	// 不使用图标以确保兼容性

	// 美化文件选择按钮
	selectFileButton.Importance = widget.MediumImportance
	// 不使用图标以确保兼容性

	// 美化开始投屏按钮 - 添加图标和更好的视觉效果
	castButton.Importance = widget.HighImportance
	// 不使用图标以确保兼容性
	castButton.OnTapped = func() {
		// 检查是否选择了设备和文件
		if len(app.devices) == 0 {
			dialog.ShowInformation("提示", "请先搜索并选择设备", app.window)
			return
		}

		// 检查是否选择了设备
		if selectedIndex < 0 {
			dialog.ShowInformation("提示", "请先选择一个设备", app.window)
			return
		}

		if app.mediaFile == "" {
			dialog.ShowInformation("提示", "请先选择要投屏的视频文件", app.window)
			return
		}

		// 显示加载对话框
		progress := dialog.NewProgress("投屏中...", "正在连接设备并准备投屏", app.window)
		progress.Show()

		// 在后台执行投屏
		go func() {
			selectedDevice := app.devices[selectedIndex]
			log.Printf("连接设备: %s, 地址: %s\n", selectedDevice.FriendlyName, selectedDevice.Location)

			// 创建设备控制器
			controller, err := dlna.NewDeviceController(selectedDevice.Location)
			if err != nil {
				log.Printf("创建设备控制器失败: %v\n", err)
				dialog.ShowError(err, app.window)
				progress.Hide()
				return
			}

			// 获取文件所在目录
			mediaDir := filepath.Dir(app.mediaFile)
			fileName := filepath.Base(app.mediaFile)

			// 启动媒体服务器并获取媒体文件的HTTP URL
			serverURL, err := app.mediaServer.Start(mediaDir)
			if err != nil {
				log.Printf("启动媒体服务器失败: %v\n", err)
				dialog.ShowError(err, app.window)
				progress.Hide()
				return
			}

			// 构建媒体文件的完整URL
			mediaURL := serverURL + "/" + fileName
			log.Printf("媒体文件URL: %s\n", mediaURL)

			// 播放媒体
			err = controller.PlayMedia(mediaURL)
			if err != nil {
				log.Printf("投屏失败: %v\n", err)
				dialog.ShowError(err, app.window)
			} else {
				log.Printf("投屏成功: %s\n", filepath.Base(app.mediaFile))
				dialog.ShowInformation("成功", "投屏成功！\n媒体文件正在通过HTTP服务器提供", app.window)
			}

			// 关闭加载对话框
			progress.Hide()
		}()
	}

	// 创建主布局 - 改进整体布局，增加更好的分组和间距
	topLayout := container.NewCenter(
		container.NewPadded(
			searchButton,
		),
	)

	// 使用自定义卡片效果包装设备列表
	deviceCount := len(app.devices)
	deviceCard := createCard(
		"可用设备",
		"找到 " + strconv.Itoa(deviceCount) + " 个设备",
		app.deviceList,
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
		container.NewHBox(
			layout.NewSpacer(),
			selectFileButton,
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

	// 设置窗口内容
	app.window.SetContent(content)
}
