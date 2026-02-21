//go:build asr_server && cgo

package asr

import (
	"context"
	"fmt"
	"xiaozhi-esp32-server-golang/constants"
	"xiaozhi-esp32-server-golang/internal/data/audio"
	"xiaozhi-esp32-server-golang/internal/domain/asr/types"
	log "xiaozhi-esp32-server-golang/logger"

	"voice_server/core"
	"voice_server/server"
)

const (
	embedMode                 = "offline"
	DefaultEmbedAsrConfigPath = "asr_server.json"
)

// EmbedConfig embed 适配器配置。
type EmbedConfig struct {
	SampleRate int
	UseVAD     bool
}

// EmbedConfigFromMap 从 map 读取 embed 配置。
func EmbedConfigFromMap(config map[string]interface{}) EmbedConfig {
	conf := EmbedConfig{
		SampleRate: audio.SampleRate,
		UseVAD:     true,
	}

	if sampleRate, ok := config["sample_rate"].(int); ok && sampleRate > 0 {
		conf.SampleRate = sampleRate
	}
	if useVAD, ok := config["use_vad"].(bool); ok {
		conf.UseVAD = useVAD
	}

	return conf
}

// AsrEmbedAdapter 适配 asr_server 进程内引擎到 AsrProvider。
type AsrEmbedAdapter struct {
	engine core.Engine
	config EmbedConfig
}

// NewAsrEmbedAdapter 创建 embed 适配器。
func NewAsrEmbedAdapter(config map[string]interface{}) (AsrProvider, error) {
	engine, err := server.SharedEngineProvider().Engine()
	if err != nil {
		return nil, err
	}

	adapterConfig := EmbedConfigFromMap(config)
	log.Debugf("embed adapter config: %+v", adapterConfig)

	return &AsrEmbedAdapter{
		engine: engine,
		config: adapterConfig,
	}, nil
}

// InitAsrServerEmbed 统一初始化内嵌 ASR 共享依赖。
func InitAsrServerEmbed(configPath string) error {
	if configPath == "" {
		configPath = DefaultEmbedAsrConfigPath
	}

	log.Infof("正在预初始化内嵌 embed 引擎，配置文件: %s", configPath)

	if err := server.SharedEngineProvider().InitForEmbed(configPath); err != nil {
		return fmt.Errorf("embed 初始化失败: %w", err)
	}
	log.Info("asr_server embed 引擎预初始化完成")
	return nil
}

// IsAsrServerEmbedInitialized 判断内嵌 ASR 引擎是否已经初始化完成。
func IsAsrServerEmbedInitialized() bool {
	_, err := server.SharedEngineProvider().Engine()
	return err == nil
}

// RequireAsrServerEmbed 确保内嵌 ASR 已初始化；已初始化时直接返回。
func RequireAsrServerEmbed(configPath string) error {
	if IsAsrServerEmbedInitialized() {
		return nil
	}
	return InitAsrServerEmbed(configPath)
}

// Process 一次性识别整段音频。
func (a *AsrEmbedAdapter) Process(pcmData []float32) (string, error) {
	if a.engine == nil {
		return "", fmt.Errorf("embed service 未初始化")
	}
	return a.recognize(pcmData)
}

// StreamingRecognize 流式收集音频，结束时返回最终识别结果。
func (a *AsrEmbedAdapter) StreamingRecognize(ctx context.Context, audioStream <-chan []float32) (chan types.StreamingResult, error) {
	if a.engine == nil {
		return nil, fmt.Errorf("embed service 未初始化")
	}
	if audioStream == nil {
		return nil, fmt.Errorf("audioStream 不能为空")
	}

	resultChan := make(chan types.StreamingResult, 1)
	go func() {
		defer close(resultChan)

		buffer := make([]float32, 0, a.config.SampleRate*3)
		for {
			select {
			case <-ctx.Done():
				return
			case pcm, ok := <-audioStream:
				if !ok {
					a.emitFinalResult(ctx, resultChan, buffer)
					return
				}
				if len(pcm) > 0 {
					buffer = append(buffer, pcm...)
				}
			}
		}
	}()

	return resultChan, nil
}

// Close 关闭资源。
func (a *AsrEmbedAdapter) Close() error {
	a.engine = nil
	return nil
}

// IsValid 检查资源是否有效。
func (a *AsrEmbedAdapter) IsValid() bool {
	return a != nil && a.engine != nil
}

func (a *AsrEmbedAdapter) recognize(pcmData []float32) (string, error) {
	if a.config.UseVAD {
		return a.engine.RecognizeWithVAD(pcmData, a.config.SampleRate)
	}
	return a.engine.RecognizeFloat32(pcmData, a.config.SampleRate)
}

func (a *AsrEmbedAdapter) emitFinalResult(ctx context.Context, resultChan chan<- types.StreamingResult, pcmData []float32) {
	text, err := a.recognize(pcmData)
	result := types.StreamingResult{
		Text:    text,
		IsFinal: true,
		AsrType: constants.AsrTypeEmbed,
		Mode:    embedMode,
	}
	if err != nil {
		log.Warnf("embed 识别失败: %v", err)
		result.Text = ""
		result.Error = err
	}

	select {
	case resultChan <- result:
	case <-ctx.Done():
	}
}
