package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// FileWatcher 文件监控结构体
type FileWatcher struct {
	FilePath    string
	Interval    time.Duration
	Offset      int64
	LastModTime time.Time
}

func main() {
	// 解析命令行参数
	var configFile string
	flag.StringVar(&configFile, "config", "", "配置文件路径")
	flag.Parse()

	// 初始化文件监控器
	watcher := &FileWatcher{
		FilePath: "/Users/zjx/GolandProjects/logai/logfile_test/app.log",
		Interval: 5 * time.Second,
	}

	if err := watcher.Init(); err != nil {
		log.Fatalf("文件监控初始化失败: %v", err)
	}

	// 启动监控goroutine
	go func() {
		ticker := time.NewTicker(watcher.Interval)
		defer ticker.Stop()

		for range ticker.C {
			if err := watcher.Watch(); err != nil {
				log.Printf("文件监控错误: %v", err)
			}
		}
	}()

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("LogAI Agent 启动成功")
	<-sigCh
	log.Printf("LogAI Agent 正在关闭...")
}

func (f *FileWatcher) Init() error {
	fileInfo, err := os.Stat(f.FilePath)
	if err != nil {
		return fmt.Errorf("文件不存在: %w", err)
	}

	f.LastModTime = fileInfo.ModTime()
	f.Offset = fileInfo.Size()
	return nil
}

func (f *FileWatcher) Watch() error {
	fileInfo, err := os.Stat(f.FilePath)
	if err != nil {
		return fmt.Errorf("文件状态获取失败: %w", err)
	}

	if fileInfo.ModTime().Before(f.LastModTime) {
		f.Offset = 0
	}

	// 初始化日志发送客户端
	sender := NewLogSender("http://localhost:8080")

	// 在Watch方法中调用发送逻辑
	if fileInfo.Size() > f.Offset {
		file, err := os.Open(f.FilePath)
		if err != nil {
			return fmt.Errorf("打开文件失败: %w", err)
		}
		defer file.Close()

		_, err = file.Seek(f.Offset, io.SeekStart)
		if err != nil {
			return fmt.Errorf("文件定位失败: %w", err)
		}

		newContent, err := io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("内容读取失败: %w", err)
		}

		log.Printf("采集到%d字节新日志，准备发送...", len(newContent))
		f.Offset = fileInfo.Size()

		if err := sender.SendLog(newContent); err != nil {
			log.Printf("日志发送失败: %v", err)
		}
	}
	return nil
}

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

func (s *LogSender) SendLog(data []byte) error {
	req, err := http.NewRequest("POST", s.ServerURL+"/put?key="+time.Now().Format(time.RFC3339Nano), bytes.NewReader(data))
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
