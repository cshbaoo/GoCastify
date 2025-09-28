package discovery

import (
	"context"
	"encoding/xml"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/koron/go-ssdp"
)

// DeviceInfo 存储发现的设备信息
type DeviceInfo struct {
	USN         string
	Location    string
	Server      string
	FriendlyName string
	UDN         string
}

// 用于解析设备XML描述中的设备信息
// 简化版结构，只提取我们需要的字段
type deviceXML struct {
	Device struct {
		FriendlyName string `xml:"friendlyName"`
		UDN          string `xml:"UDN"`
	} `xml:"device"`
}

// SearchDevices 搜索局域网中的DLNA设备
func SearchDevices(timeout time.Duration) ([]DeviceInfo, error) {
	// 默认使用background上下文
	return SearchDevicesWithContext(context.Background(), timeout)
}

// SearchDevicesWithContext 支持取消的设备搜索函数
func SearchDevicesWithContext(ctx context.Context, timeout time.Duration) ([]DeviceInfo, error) {
	// 创建一个带超时的上下文
	searchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 存储所有搜索到的设备，使用UDN作为键进行去重
	allDevices := make(map[string]DeviceInfo)
	// 用于跟踪已经尝试获取详细信息的Location URL
	processedLocations := make(map[string]bool)

	// 定义要搜索的多种设备类型，增加发现成功率
	deviceTypes := []string{
		"ssdp:all", // 搜索所有SSDP设备
		"urn:schemas-upnp-org:device:MediaRenderer:1", // 标准媒体渲染器
		"urn:schemas-upnp-org:device:MediaRenderer:2", // 较新的媒体渲染器版本
	}

	// 使用WaitGroup等待所有搜索和处理完成
	var wg sync.WaitGroup
	var resultMutex sync.Mutex
	// 使用信号量限制并发数量，避免过多的并发请求
	semaphore := make(chan struct{}, 5) // 限制最多5个并发请求

	// 搜索结果处理函数
	processResult := func(res ssdp.Service) {
		defer func() {
			<-semaphore // 释放信号量
			wg.Done()
		}()

		// 检查是否已取消
		if searchCtx.Err() != nil {
			return
		}

		// 创建一个带超时的上下文用于单个设备详情请求
		detailCtx, cancelDetail := context.WithTimeout(searchCtx, 3*time.Second)
		defer cancelDetail()

		// 获取设备详情
		detail, err := getDeviceDetailsWithContext(detailCtx, res.Location)
		if err != nil {
			log.Printf("获取设备详情失败(%s): %v\n", res.Location, err)
			return
		}

		// 创建设备信息
		device := DeviceInfo{
			USN:         res.USN,
			Location:    res.Location,
			Server:      res.Server,
			FriendlyName: detail.Device.FriendlyName,
			UDN:         detail.Device.UDN,
		}

		// 使用UDN作为键进行去重
		resultMutex.Lock()
		allDevices[device.UDN] = device
		resultMutex.Unlock()
	}

	// 对每种设备类型进行搜索
	for _, deviceType := range deviceTypes {
		// 检查是否已取消
		if searchCtx.Err() != nil {
			log.Printf("搜索上下文已取消(%v)，停止新的搜索", searchCtx.Err())
			break
		}

		log.Printf("开始搜索设备类型: %s，超时时间: %v\n", deviceType, timeout/2)

		// 执行搜索
		results, err := ssdp.Search(deviceType, int((timeout/2).Seconds()), "")
		if err != nil {
			log.Printf("搜索设备类型 %s 失败: %v\n", deviceType, err)
			continue
		}

		// 处理每个搜索结果
		for _, res := range results {
			// 避免重复处理同一Location
			resultMutex.Lock()
			if processedLocations[res.Location] {
				resultMutex.Unlock()
				continue
			}
			processedLocations[res.Location] = true
			resultMutex.Unlock()

			// 等待获取信号量
			semaphore <- struct{}{}
			wg.Add(1)
			go processResult(res)
		}
	}

	// 等待所有搜索和处理完成
	doneChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneChan)
	}()

	// 等待所有处理完成或上下文取消
	select {
	case <-doneChan:
		// 将map转换为slice
		devices := make([]DeviceInfo, 0, len(allDevices))
		for _, device := range allDevices {
			devices = append(devices, device)
		}
		return devices, nil
	case <-searchCtx.Done():
		// 如果超时或取消，返回已找到的设备
		devices := make([]DeviceInfo, 0, len(allDevices))
		for _, device := range allDevices {
			devices = append(devices, device)
		}
		// 如果已经找到了设备，就返回这些设备而不是错误
		if len(devices) > 0 {
			return devices, nil
		}
		return nil, searchCtx.Err()
	}
}

// getDeviceDetails 从设备的Location URL获取详细信息
func getDeviceDetails(location string) (*deviceXML, error) {
	// 创建默认上下文
	ctx := context.Background()
	return getDeviceDetailsWithContext(ctx, location)
}

// getDeviceDetailsWithContext 使用带上下文的HTTP请求获取设备详细信息
func getDeviceDetailsWithContext(ctx context.Context, location string) (*deviceXML, error) {
	log.Printf("正在获取设备详情: %s\n", location)
	
	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "GET", location, nil)
	if err != nil {
		log.Printf("创建HTTP请求失败: %v\n", err)
		return nil, err
	}

	// 设置HTTP请求的超时时间
	client := http.Client{
		Timeout: 3 * time.Second, // 明确设置超时时间
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("HTTP请求失败: %v\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	log.Printf("获取设备详情成功，状态码: %d\n", resp.StatusCode)
	
	// 读取响应体
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("读取响应体失败: %v\n", err)
		return nil, err
	}

	// 解析XML数据
	var deviceXML deviceXML
	err = xml.Unmarshal(data, &deviceXML)
	if err != nil {
		log.Printf("解析XML失败: %v\n\n响应数据预览: %s...\n", err, string(data[:min(200, len(data))]))
		return nil, err
	}

	log.Printf("成功解析设备详情: 设备名称='%s', UDN='%s'\n", deviceXML.Device.FriendlyName, deviceXML.Device.UDN)
	return &deviceXML, nil
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}