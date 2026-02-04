package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"xiaozhi-esp32-server-golang/internal/app/server"
	log "xiaozhi-esp32-server-golang/logger"

	"github.com/spf13/viper"
)

func main() {
	// 解析命令行参数
	configFile := flag.String("c", "config/config.yaml", "配置文件路径")
	managerEnable := flag.Bool("manager-enable", false, "是否启用内嵌 manager")
	managerConfig := flag.String("manager-config", "", "manager 配置文件路径，启用时可选，默认 manager/backend/config/config.json")
	asrEnable := flag.Bool("asr-enable", false, "是否启用内嵌 asr_server")
	asrConfig := flag.String("asr-config", "", "asr_server 配置文件路径，启用时可选，默认 asr_server/config.json")
	flag.Parse()

	if *configFile == "" {
		fmt.Println("配置文件路径不能为空")
		return
	}

	// 先启动 manager，再 Init，否则 Init 里 updateConfigFromAPI 会一直连不上 manager 导致卡死
	if *managerEnable {
		StartManagerHTTP(*managerConfig)
	}
	if *asrEnable {
		StartAsrServerHTTP(*asrConfig)
	}
	err := Init(*configFile)
	if err != nil {
		return
	}

	// 根据配置启动pprof服务
	if viper.GetBool("server.pprof.enable") {
		pprofPort := viper.GetInt("server.pprof.port")
		go func() {
			log.Infof("启动pprof服务，端口: %d", pprofPort)
			if err := http.ListenAndServe(fmt.Sprintf(":%d", pprofPort), nil); err != nil {
				log.Errorf("pprof服务启动失败: %v", err)
			}
		}()
		log.Infof("pprof地址: http://localhost:%d/debug/pprof/", pprofPort)
	} else {
		log.Info("pprof服务已禁用")
	}

	// 创建服务器
	appInstance := server.NewApp()
	appInstance.Run()

	// 阻塞监听退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	log.Info("服务器已启动，按 Ctrl+C 退出")
	<-quit

	log.Info("正在关闭服务器...")

	// 停止周期性配置更新服务
	StopPeriodicConfigUpdate()
	if *managerEnable {
		StopManagerHTTP()
	}
	if *asrEnable {
		StopAsrServerHTTP()
	}

	log.Info("服务器已关闭")
}
