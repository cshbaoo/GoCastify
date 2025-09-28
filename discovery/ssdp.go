package discovery

import (
	"context"
	"encoding/xml"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/koron/go-ssdp"
	"GoCastify/interfaces"
	"GoCastify/types"
)

// SSDPDiscoverer 基于SSDP协议的设备发现器
// 实现了interfaces.DeviceDiscoverer接口

type SSDPDiscoverer struct {
	devices        []types.DeviceInfo
	devicesMutex   sync.RWMutex
}

// NewSSDPDiscoverer 创建一个新的SSDP设备发现器
func NewSSDPDiscoverer() interfaces.DeviceDiscoverer {
	return &SSDPDiscoverer{}
}

// StartSearchWithContext 开始搜索DLNA设备
func (sd *SSDPDiscoverer) StartSearchWithContext(ctx context.Context, onDeviceFound func(types.DeviceInfo)) error {
	// 重置设备列表
	sd.devicesMutex.Lock()
	sd.devices = []types.DeviceInfo{}
	sd.devicesMutex.Unlock()

	// 创建一个带超时的上下文
	timeout := 10 * time.Second
	searchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 存储所有搜索到的设备，使用UDN作为键进行去重
	allDevices := make(map[string]types.DeviceInfo)
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
		device := types.DeviceInfo{
			FriendlyName: detail.Device.FriendlyName,
			Location:     res.Location,
			Manufacturer: extractManufacturerFromServer(res.Server),
			ModelName:    extractModelFromServer(res.Server),
		}

		// 使用UDN作为键进行去重
		udn := detail.Device.UDN
		resultMutex.Lock()
		if _, exists := allDevices[udn]; !exists {
			allDevices[udn] = device
			// 如果提供了回调函数，调用它
			if onDeviceFound != nil {
				onDeviceFound(device)
			}
		}
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
		devices := make([]types.DeviceInfo, 0, len(allDevices))
		for _, device := range allDevices {
			devices = append(devices, device)
		}
		
		// 更新设备列表
		sd.devicesMutex.Lock()
		sd.devices = devices
		sd.devicesMutex.Unlock()
		
		return nil
	case <-searchCtx.Done():
		// 如果超时或取消，返回已找到的设备
		devices := make([]types.DeviceInfo, 0, len(allDevices))
		for _, device := range allDevices {
			devices = append(devices, device)
		}
		
		// 更新设备列表
		sd.devicesMutex.Lock()
		sd.devices = devices
		sd.devicesMutex.Unlock()
		
		// 如果已经找到了设备，就返回成功
		if len(devices) > 0 {
			return nil
		}
		return searchCtx.Err()
	}
}

// GetDevices 获取已发现的设备列表
func (sd *SSDPDiscoverer) GetDevices() []types.DeviceInfo {
	sd.devicesMutex.RLock()
	defer sd.devicesMutex.RUnlock()
	
	// 返回设备列表的副本
	devicesCopy := make([]types.DeviceInfo, len(sd.devices))
	copy(devicesCopy, sd.devices)
	return devicesCopy
}

// 用于解析设备XML描述中的设备信息
// 简化版结构，只提取我们需要的字段
type deviceXML struct {
	Device struct {
		FriendlyName string `xml:"friendlyName"`
		UDN          string `xml:"UDN"`
	} `xml:"device"`
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
	data, err := io.ReadAll(resp.Body)
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

// extractManufacturerFromServer 从Server头中提取制造商信息
func extractManufacturerFromServer(server string) string {
	// 简化实现，实际项目中可能需要更复杂的解析逻辑
	return "Unknown"
}

// extractModelFromServer 从Server头中提取型号信息
func extractModelFromServer(server string) string {
	// 简化实现，实际项目中可能需要更复杂的解析逻辑
	return "Unknown"
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}