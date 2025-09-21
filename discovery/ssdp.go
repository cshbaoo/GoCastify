package discovery

import (
	"context"
	"encoding/xml"
	"io/ioutil"
	"log"
	"net/http"
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
// getDeviceDetails 从设备的Location URL获取详细信息
func getDeviceDetails(location string) (*deviceXML, error) {
	// 设置HTTP请求的超时时间
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(location)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应体
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 解析XML数据
	var deviceXML deviceXML
	err = xml.Unmarshal(data, &deviceXML)
	if err != nil {
		return nil, err
	}

	return &deviceXML, nil
}

func SearchDevicesWithContext(ctx context.Context, timeout time.Duration) ([]DeviceInfo, error) {
	// 创建一个带超时的上下文
	searchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 在单独的goroutine中执行搜索并处理结果
	resultsChan := make(chan []DeviceInfo, 1)
	errChan := make(chan error, 1)

	go func() {
		// 存储所有搜索到的设备，使用UDN作为键进行去重
		allDevices := make(map[string]DeviceInfo)
		// 用于跟踪已经尝试获取详细信息的Location URL
		processedLocations := make(map[string]bool)

		// 定义要搜索的多种设备类型，增加发现成功率
		deviceTypes := []string{
			"urn:schemas-upnp-org:device:MediaRenderer:1", // 标准媒体渲染器
			"urn:schemas-upnp-org:device:MediaRenderer:2", // 较新的媒体渲染器版本
			"ssdp:all", // 搜索所有SSDP设备
		}

		// 对每种设备类型进行搜索
		for _, deviceType := range deviceTypes {
			// 检查是否已取消
			if searchCtx.Err() != nil {
				log.Printf("搜索上下文已取消(%v)，但继续收集已找到的设备", searchCtx.Err())
				// 不立即返回，而是继续处理已找到的设备
				break
			}

			log.Printf("开始搜索设备类型: %s，超时时间: %v\n", deviceType, timeout/2)

			// 执行搜索
			results, err := ssdp.Search(deviceType, int((timeout/2).Seconds()), "")
			if err != nil {
				log.Printf("搜索设备类型 %s 失败: %v\n", deviceType, err)
				// 继续搜索下一种类型，不立即返回错误
				continue
			}

			log.Printf("搜索设备类型 %s 完成，收到 %d 个响应\n", deviceType, len(results))

			// 处理搜索结果
			for _, r := range results {
				// 检查是否已取消，但即使取消也继续处理已收到的结果
				if searchCtx.Err() != nil {
					log.Printf("搜索上下文已取消(%v)，但继续处理当前结果", searchCtx.Err())
				}

				// 处理设备信息，尝试从Location获取详细信息
				tdevice := DeviceInfo{
					USN:      r.USN,
					Location: r.Location,
					Server:   r.Server,
				}

				// 尝试从Location URL获取设备详细信息
				// 只对每个Location URL处理一次，避免重复请求
				if !processedLocations[r.Location] {
					processedLocations[r.Location] = true
					deviceDetails, err := getDeviceDetails(r.Location)
					if err == nil && deviceDetails != nil {
						// 成功获取到详细信息
						udn := deviceDetails.Device.UDN
						friendlyName := deviceDetails.Device.FriendlyName
						
						// 如果获取到了UDN，使用UDN作为去重键
						if udn != "" {
							tdevice.UDN = udn
							// 检查是否已存在该UDN的设备
							if _, exists := allDevices[udn]; exists {
								// 设备已存在，不重复添加
								continue
							}
						} else {
							// 没有UDN，使用USN作为备选
							udn = r.USN
							if _, exists := allDevices[udn]; exists {
								continue
							}
							}

						// 如果获取到了FriendlyName，使用它
						if friendlyName != "" {
							tdevice.FriendlyName = friendlyName
						} else {
							// 没有获取到FriendlyName，使用Server作为备选
							tdevice.FriendlyName = r.Server
						}

						// 添加到结果集
						allDevices[udn] = tdevice
						log.Printf("发现设备: %s, UDN: %s, Location: %s\n", tdevice.FriendlyName, tdevice.UDN, tdevice.Location)
					} else {
						// 无法获取详细信息，使用基本信息并以USN为键
						tdevice.FriendlyName = r.Server
						if _, exists := allDevices[r.USN]; !exists {
							allDevices[r.USN] = tdevice
							log.Printf("无法获取设备详情，使用基本信息: %s, USN: %s\n", tdevice.FriendlyName, tdevice.USN)
						}
					}
				} else {
					// 此Location已经处理过，跳过
					log.Printf("Location已处理，跳过: %s\n", r.Location)
				}
			}
		}

		// 将map转换为slice
		devices := make([]DeviceInfo, 0, len(allDevices))
		for _, device := range allDevices {
			devices = append(devices, device)
		}

		log.Printf("所有搜索完成，共发现 %d 个唯一设备，准备返回结果\n", len(devices))

		// 添加详细的设备信息日志
		for i, device := range devices {
			log.Printf("设备 %d: USN=%s, FriendlyName=%s, Location=%s\n", i+1, device.USN, device.FriendlyName, device.Location)
		}

		// 返回找到的设备，无论上下文是否已取消
		resultsChan <- devices
	}()

	// 等待搜索结果或取消信号
	select {
	case <-searchCtx.Done():
		// 上下文已取消或超时，但我们仍然等待goroutine完成并返回已找到的设备
		log.Printf("主搜索上下文已完成(%v)，等待收集已找到的设备\n", searchCtx.Err())
		// 直接等待goroutine完成，不再设置1秒超时
		// 因为我们已经确保goroutine会发送结果，无论上下文是否取消
		select {
		case devices := <-resultsChan:
			// 即使超时，也返回已找到的设备
			log.Printf("返回收集到的 %d 个设备\n", len(devices))
			return devices, nil
		}
	case err := <-errChan:
		// 发生错误
		return nil, err
	case devices := <-resultsChan:
		// 搜索完成，返回结果
		return devices, nil
	}
}