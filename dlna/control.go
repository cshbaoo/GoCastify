package dlna

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

// DeviceController 用于控制DLNA设备
type DeviceController struct {
	ControlURL string
	EventURL   string
	DeviceInfo DeviceInfo
}

// DeviceInfo 存储DLNA设备信息
type DeviceInfo struct {
	FriendlyName string
	Manufacturer string
	ModelName    string
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

// NewDeviceController 创建一个新的设备控制器
func NewDeviceController(location string) (*DeviceController, error) {
	// 获取设备描述
	desc, err := getDeviceDescription(location)
	if err != nil {
		return nil, err
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

	// 构建完整的控制URL
	baseURL := location[:strings.LastIndex(location, "/")+1]
	fullControlURL := baseURL + strings.TrimPrefix(controlURL, "/")

	controller := &DeviceController{
		ControlURL: fullControlURL,
		EventURL:   eventURL,
		DeviceInfo: DeviceInfo{
			FriendlyName: desc.Device.FriendlyName,
			Manufacturer: desc.Device.Manufacturer,
			ModelName:    desc.Device.ModelName,
		},
	}

	return controller, nil
}

// getDeviceDescription 获取设备描述
func getDeviceDescription(location string) (*deviceDescription, error) {
	client := http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(location)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	desc := &deviceDescription{}
	err = xml.Unmarshal(body, desc)
	if err != nil {
		return nil, err
	}

	return desc, nil
}

// PlayMedia 播放指定的媒体文件
func (dc *DeviceController) PlayMedia(mediaURL string) error {
	// 设置AVTransport
	setAVTransportXML := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:SetAVTransportURI xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <InstanceID>0</InstanceID>
      <CurrentURI>%s</CurrentURI>
      <CurrentURIMetaData></CurrentURIMetaData>
    </u:SetAVTransportURI>
  </s:Body>
</s:Envelope>`, mediaURL)

	// 发送SetAVTransportURI请求
	err := dc.sendSOAPRequest("SetAVTransportURI", setAVTransportXML)
	if err != nil {
		return err
	}

	// 增加延迟时间到2秒，让设备有更充分的时间准备播放
	time.Sleep(2 * time.Second)

	// 播放请求XML
	playXML := `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:Play xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <InstanceID>0</InstanceID>
      <Speed>1</Speed>
    </u:Play>
  </s:Body>
</s:Envelope>`

	// 发送Play请求
	return dc.sendSOAPRequest("Play", playXML)
}

// sendSOAPRequest 发送SOAP请求
func (dc *DeviceController) sendSOAPRequest(action string, body string) error {
	client := http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest("POST", dc.ControlURL, bytes.NewBufferString(body))
	if err != nil {
		return err
	}

	// 设置SOAP请求头
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", fmt.Sprintf(`"urn:schemas-upnp-org:service:AVTransport:1#%s"`, action))

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		respBody, _ := ioutil.ReadAll(resp.Body)
		log.Printf("SOAP请求失败: %s, 状态码: %d, 响应: %s\n", action, resp.StatusCode, string(respBody))
		return fmt.Errorf("SOAP请求失败: %s, 状态码: %d", action, resp.StatusCode)
	}

	log.Printf("SOAP请求成功: %s\n", action)
	return nil
}