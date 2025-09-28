package dlna

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"GoCastify/interfaces"
	"GoCastify/types"
)

// DLNA相关常量定义
const (
	// UPnP服务类型
	uPNPAVTransportService = "urn:schemas-upnp-org:service:AVTransport:1"
	// 默认HTTP请求超时
	defaultHTTPTimeout = 5 * time.Second
	// 设备准备播放所需的延迟时间
	deviceReadyDelay = 2 * time.Second
)

// XML模板定义为常量
const (
	// SetAVTransportURI请求模板
	setAVTransportXMLTemplate = `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:SetAVTransportURI xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <InstanceID>0</InstanceID>
      <CurrentURI>%s</CurrentURI>
      <CurrentURIMetaData></CurrentURIMetaData>
    </u:SetAVTransportURI>
  </s:Body>
</s:Envelope>`

	// Play请求模板
	playXML = `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:Play xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <InstanceID>0</InstanceID>
      <Speed>1</Speed>
    </u:Play>
  </s:Body>
</s:Envelope>`
)

// DeviceController 用于控制DLNA设备
// 实现了interfaces.DLNAController接口
type DeviceController struct {
	ControlURL      string
	EventURL        string
	deviceInfo      types.DeviceInfo
	subscriptionMgr *SubscriptionManager
}

// ParseDeviceDescription 解析设备描述XML
type deviceDescription struct {
	Device struct {
		FriendlyName string `xml:"friendlyName"`
		Manufacturer string `xml:"manufacturer"`
		ModelName    string `xml:"modelName"`
		ServiceList struct {
			Service []struct {
				ServiceType string `xml:"serviceType"`
				ControlURL  string `xml:"controlURL"`
				EventSubURL string `xml:"eventSubURL"`
			} `xml:"service"`
		} `xml:"serviceList"`
	} `xml:"device"`
}

// NewDeviceControllerWithContext 创建一个带上下文支持的设备控制器
func NewDeviceControllerWithContext(ctx context.Context, location string) (interfaces.DLNAController, error) {
	// 获取设备描述
	desc, err := getDeviceDescriptionWithContext(ctx, location)
	if err != nil {
		return nil, fmt.Errorf("获取设备描述失败: %w", err)
	}

	// 查找AVTransport服务
	controlURL := ""
	eventURL := ""
	for _, service := range desc.Device.ServiceList.Service {
		if strings.Contains(service.ServiceType, "AVTransport") {
			controlURL = service.ControlURL
			eventURL = service.EventSubURL
			break
		}
	}

	if controlURL == "" {
		return nil, fmt.Errorf("未找到AVTransport服务")
	}

	// 构建完整的控制URL
	baseURL := location[:strings.LastIndex(location, "/")+1]
	fullControlURL := baseURL + strings.TrimPrefix(controlURL, "/")

	controller := &DeviceController{
		ControlURL: fullControlURL,
		EventURL:   eventURL,
		deviceInfo: types.DeviceInfo{
			FriendlyName: desc.Device.FriendlyName,
			Manufacturer: desc.Device.Manufacturer,
			ModelName:    desc.Device.ModelName,
			Location:     location,
		},
	}

	// 初始化订阅管理器
	controller.subscriptionMgr = newSubscriptionManager(controller)

	return controller, nil
}

// NewDeviceController 创建一个新的设备控制器
func NewDeviceController(location string) (interfaces.DLNAController, error) {
	return NewDeviceControllerWithContext(context.Background(), location)
}

// GetDeviceInfo 获取设备信息
func (dc *DeviceController) GetDeviceInfo() types.DeviceInfo {
	return dc.deviceInfo
}

// getDeviceDescriptionWithContext 使用带上下文的HTTP请求获取设备描述
func getDeviceDescriptionWithContext(ctx context.Context, location string) (*deviceDescription, error) {
	client := http.Client{
		Timeout: defaultHTTPTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", location, nil)
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取设备描述失败，状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	desc := &deviceDescription{}
	err = xml.Unmarshal(body, desc)
	if err != nil {
		// 仅记录前200个字符，避免日志过长
		dataPreview := string(body[:min(200, len(body))])
		return nil, fmt.Errorf("解析XML失败: %w\n响应数据预览: %s...", err, dataPreview)
	}

	return desc, nil
}

// getDeviceDescription 获取设备描述
func getDeviceDescription(location string) (*deviceDescription, error) {
	return getDeviceDescriptionWithContext(context.Background(), location)
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// PlayMediaWithContext 带上下文支持的媒体播放函数
func (dc *DeviceController) PlayMediaWithContext(ctx context.Context, mediaURL string) error {
	// 设置AVTransport
	setAVTransportXML := fmt.Sprintf(setAVTransportXMLTemplate, mediaURL)

	// 发送SetAVTransportURI请求
	err := dc.sendSOAPRequestWithContext(ctx, "SetAVTransportURI", setAVTransportXML)
	if err != nil {
		return fmt.Errorf("设置AVTransport失败: %w", err)
	}

	// 增加延迟时间，让设备有更充分的时间准备播放
	// 检查上下文是否已取消
	sleepCtx, cancel := context.WithTimeout(ctx, deviceReadyDelay)
	defer cancel()
	select {
	case <-sleepCtx.Done():
		// 上下文已取消或超时
		return sleepCtx.Err()
	case <-time.After(deviceReadyDelay):
		// 延迟结束，继续执行
	}

	// 发送Play请求
	err = dc.sendSOAPRequestWithContext(ctx, "Play", playXML)
	if err != nil {
		return err
	}

	// 启动事件订阅
	if dc.subscriptionMgr != nil {
		dc.subscriptionMgr.startSubscription(ctx)
	}

	return nil
}

// PlayMedia 播放指定的媒体文件（兼容旧接口）
func (dc *DeviceController) PlayMedia(mediaURL string) error {
	return dc.PlayMediaWithContext(context.Background(), mediaURL)
}

// SubscriptionManager 管理DLNA事件订阅
// 这是一个内部组件，负责处理设备事件通知
type SubscriptionManager struct {
	controller *DeviceController
	cancelFunc context.CancelFunc
}

// newSubscriptionManager 创建一个新的订阅管理器
func newSubscriptionManager(controller *DeviceController) *SubscriptionManager {
	return &SubscriptionManager{
		controller: controller,
	}
}

// startSubscription 开始订阅设备事件
func (sm *SubscriptionManager) startSubscription(ctx context.Context) {
	// 如果已经有活跃的订阅，先取消
	if sm.cancelFunc != nil {
		sm.cancelFunc()
	}

	// 创建一个子上下文用于订阅
	subCtx, cancel := context.WithCancel(ctx)
	sm.cancelFunc = cancel

	// 在后台启动订阅处理
	go sm.handleSubscription(subCtx)
}

// handleSubscription 处理事件订阅
func (sm *SubscriptionManager) handleSubscription(ctx context.Context) {
	// 简化实现，实际项目中可能需要实现真正的UPnP事件订阅
	// 此处我们只是定期检查上下文是否已取消

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Printf("开始事件订阅监控: %s", sm.controller.deviceInfo.FriendlyName)

	for {
		select {
		case <-ctx.Done():
			log.Printf("停止事件订阅监控: %v", ctx.Err())
			return
		case <-ticker.C:
			// 定期检查
		}
	}
}

// sendSOAPRequestWithContext 带上下文支持的SOAP请求发送函数
func (dc *DeviceController) sendSOAPRequestWithContext(ctx context.Context, action string, body string) error {
	client := http.Client{
		Timeout: defaultHTTPTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, "POST", dc.ControlURL, bytes.NewBufferString(body))
	if err != nil {
		return fmt.Errorf("创建SOAP请求失败: %w", err)
	}

	// 设置SOAP请求头
	soapAction := fmt.Sprintf(`"%s#%s"`, uPNPAVTransportService, action)
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", soapAction)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送SOAP请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		// 读取响应体以获取更多错误信息
		respBody, _ := io.ReadAll(resp.Body)
		// 仅记录前200个字符，避免日志过长
		respBodyPreview := string(respBody[:min(200, len(respBody))])
		log.Printf("SOAP请求失败: %s, 状态码: %d, 响应预览: %s...\n", action, resp.StatusCode, respBodyPreview)
		return fmt.Errorf("SOAP请求失败: %s, 状态码: %d", action, resp.StatusCode)
	}

	log.Printf("SOAP请求成功: %s\n", action)
	return nil
}

// sendSOAPRequest 发送SOAP请求
func (dc *DeviceController) sendSOAPRequest(action string, body string) error {
	return dc.sendSOAPRequestWithContext(context.Background(), action, body)
}