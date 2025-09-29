# GoCastify

GoCastify is a DLNA casting tool developed in Go language that allows users to easily cast local media files (videos, audios) to DLNA-compatible devices such as smart TVs, speakers, etc.

## Features

- 📺 Support video file casting
- 🎵 Support audio file casting
- 📝 Support subtitle file selection and casting
- 🔍 Automatic discovery of DLNA devices within the local network
- 🎯 Support multi-audio track selection
- 💻 Clean and intuitive user interface
- 🌐 Built-in HTTP media server
- ⚡ Efficient media transcoding functionality (based on FFmpeg)

## Tech Stack

- **Backend Language**: Go
- **UI Framework**: fyne.io
- **Media Transcoding**: FFmpeg
- **Network Protocol**: DLNA/UPnP
- **Build Tool**: Go Modules

## Installation

### Prerequisites

- Install Go 1.18 or higher
- Install FFmpeg (for media transcoding)

### Compilation and Installation

```bash
# Clone the repository
git clone https://github.com/cshbaoo/GoCastify.git
cd GoCastify

# Build the project
go build

# Run the application
./GoCastify
```

## Usage Instructions

1. Start the GoCastify application
2. The application will automatically search for DLNA devices in the local network
3. Click the "Select File" button to choose the media file you want to cast
4. Optional: Click the "Select Audio Track" button to choose the audio track you want to use
5. Optional: Click the "Select Subtitle" button to choose the subtitle file you want to use
6. Select the target DLNA device
7. Click the "Start Casting" button to begin playback

## Project Architecture

GoCastify adopts a clear layered architecture and interface design, with main components including:

### Core Modules

- **app/** - Core logic and state management of the application
- **interfaces/** - Defines interfaces for interactions between all modules to ensure decoupling
- **types/** - Defines shared data types to ensure type consistency throughout the project

### Functional Modules

- **discovery/** - Responsible for DLNA device discovery, implements the `interfaces.DeviceDiscoverer` interface
- **dlna/** - Provides DLNA device control functionality, implements the `interfaces.DLNAController` interface
- **server/** - Built-in HTTP media server, implements the `interfaces.MediaServer` interface
- **transcoder/** - Media transcoding functionality, based on FFmpeg, implements the `interfaces.MediaTranscoder` interface
- **ui/** - User interface implementation

### Project Structure

```
GoCastify/
├── app/
│   └── app.go     # Application main logic implementation
├── discovery/
│   └── ssdp.go    # SSDP protocol implementation, DLNA device discovery
├── dlna/
│   └── control.go # DLNA device control functionality
├── interfaces/
│   └── interfaces.go # Core interface definitions
├── server/
│   └── media_server.go # HTTP media server implementation
├── transcoder/
│   └── transcoder.go # FFmpeg-based transcoding implementation
├── types/
│   └── types.go   # Shared data type definitions
├── ui/
│   └── ui.go      # User interface implementation
├── go.mod         # Go module definition
├── go.sum         # Dependency version locking
└── main.go        # Program entry point
```

## Interface Design

The project adopts a clear interface design, with main interfaces including:

### DLNAController
- `PlayMediaWithContext(ctx context.Context, mediaURL string) error` - Media playback function with context support
- `GetDeviceInfo() types.DeviceInfo` - Get device information

### MediaServer
- `Start(mediaDir string) (string, error)` - Start the media server, return server URL
- `Stop() error` - Stop the media server
- `ServeHTTP(w http.ResponseWriter, r *http.Request)` - Handle HTTP requests

### MediaTranscoder
- `GetSubtitleTracks(filePath string) ([]types.SubtitleTrack, error)` - Get subtitle track information from media files
- `GetAudioTracks(filePath string) ([]types.AudioTrack, error)` - Get audio track information from media files
- `TranscodeToMp4(inputFile string, subtitleTrackIndex int, audioTrackIndex int) (string, error)` - Transcode media files to MP4 format
- `StreamTranscode(inputFile string, subtitleTrackIndex int, audioTrackIndex int) (string, error)` - Real-time streaming transcoding
- `Cleanup() error` - Clean up temporary files and resources

### DeviceDiscoverer
- `StartSearchWithContext(ctx context.Context, onDeviceFound func(types.DeviceInfo)) error` - Start searching for DLNA devices
- `GetDevices() []types.DeviceInfo` - Get the list of discovered devices

## Best Practices

1. **Use Context-Supported Methods** - Prefer using methods with the `WithContext` suffix, as they support timeout control and cancellation operations

2. **Device Discovery** - Use the `DeviceDiscoverer` interface for device search, avoiding direct use of specific implementations

3. **Media Server** - The media server receives transcoders through dependency injection, facilitating testing and replacing implementations

4. **Type Safety** - The entire project uses the unified `types.DeviceInfo` type to represent device information

## Development Instructions

### Dependency Management

The project uses Go Modules for dependency management:

```bash
# Install dependencies
go mod tidy

# Update dependencies
go get -u
```

### Development Process

1. Fork and clone the repository
2. Create a new feature branch
3. Implement features or fix bugs
4. Ensure the code can compile normally
5. Submit code and create a Pull Request

## Notes

- **Deprecation Warning** - The `StartCasting` method has been deprecated, please use the `StartCastingWithContext` method for better control and cancellation functionality
- **Performance Optimization** - Device search adopts concurrent processing and semaphore limits to avoid excessive concurrent requests
- **Resource Management** - Ensure the `Cleanup` method is properly called to release transcoder resources
- **Error Handling** - All critical operations have detailed error handling and logging

## License

This project adopts a custom open-source license, allowing personal learning and non-commercial use, but prohibiting any commercial use. Please refer to the LICENSE file for details