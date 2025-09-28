package types

// DeviceInfo 存储DLNA设备信息
type DeviceInfo struct {
	FriendlyName string
	Manufacturer string
	ModelName    string
	Location     string
}

// SubtitleTrack 表示媒体文件中的字幕轨道信息
type SubtitleTrack struct {
	Index     int
	Language  string
	Title     string
	IsDefault bool
}

// AudioTrack 表示媒体文件中的音频轨道信息
type AudioTrack struct {
	Index     int
	Language  string
	Title     string
	CodecName string
	IsDefault bool
}