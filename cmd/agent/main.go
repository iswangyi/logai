package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/logai/pkg/collector"
	"k8s.io/client-go/kubernetes"
)

// FileWatcher 文件监控结构体
type KubernetesCollector struct {
	clientset   *kubernetes.Clientset
	logBasePath string
	logCh       chan string
	namespace   string
}

func main() {
	// 解析命令行参数
	var configFile string
	flag.StringVar(&configFile, "config", "", "配置文件路径")
	flag.Parse()

	// 初始化文件监控器
	collector, err := collector.NewKubernetesCollector("", "/var/log/pods", "kube-system")
	if err != nil {
		log.Fatalf("Kubernetes收集器初始化失败: %v", err)
	}

	go func() {
		if err := collector.Start(context.Background()); err != nil {
			log.Fatalf("日志收集启动失败: %v", err)
		}
	}()

	// 启动监控goroutine
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			// 触发收集器检查逻辑
			log.Printf("执行定期Kubernetes日志收集")
		}
	}()

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("LogAI Agent 启动成功")
	<-sigCh
	log.Printf("LogAI Agent 正在关闭...")
}

// 删除Init方法
// 删除FileWatcher结构体定义
// 删除Watch方法

// 新增HTTP客户端结构体
type LogSender struct {
	ServerURL string
	Client    *http.Client
}

func NewLogSender(serverURL string) *LogSender {
	return &LogSender{
		ServerURL: serverURL,
		Client:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *LogSender) SendLog(containerName string, data []byte) error {
	req, err := http.NewRequest("POST", s.ServerURL+"/put?key="+time.Now().Format(time.RFC3339Nano), bytes.NewReader(data))
	if err == nil {
		req.Header.Set("X-Container-Name", containerName)
	}
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := s.Client.Do(req)
	if err != nil {
		return fmt.Errorf("请求发送失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("服务器返回错误状态码: %d 响应内容: %s", resp.StatusCode, string(body))
	}
	//响应码201，200则发送成功
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		log.Printf("日志发送成功")
	}
	return nil
}
