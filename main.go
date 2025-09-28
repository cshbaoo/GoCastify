package main

import (
	"log"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"GoCastify/app"
	"GoCastify/ui"
)

func main() {
	// 创建Fyne应用，使用唯一ID来支持Preferences API
	myApp := fyneapp.NewWithID("com.gocastify.dlnacast")
	
	// 创建主窗口
	window := myApp.NewWindow("GoCastify - DLNA投屏工具")
	// 设置窗口大小
	window.Resize(fyne.NewSize(800, 600))

	// 初始化应用程序核心逻辑
	appInstance, err := app.NewApp(myApp, window)
	if err != nil {
		log.Printf("初始化应用失败: %v\n", err)
		return
	}

	// 构建用户界面
	content := ui.BuildUI(appInstance)
	// 设置窗口内容
	window.SetContent(content)

	// 运行应用程序
	window.ShowAndRun()
}
