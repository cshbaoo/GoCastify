package main

import (
	"log"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"GoCastify/app"
	"GoCastify/ui"
)

func main() {
	// 创建Fyne应用
	myApp := fyneapp.New()
	// 创建主窗口
	window := myApp.NewWindow("Go2TV - DLNA投屏工具")
	// 设置窗口大小
	window.Resize(fyne.NewSize(800, 600))

	// 初始化应用程序核心逻辑
	appInstance, err := app.NewApp(window)
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
