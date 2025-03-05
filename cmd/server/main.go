package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/logai/pkg/storage"
)

func main() {
	// 解析命令行参数
	var (
		configFile string
		listenAddr string
	)
	flag.StringVar(&configFile, "config", "", "配置文件路径")
	flag.StringVar(&listenAddr, "addr", ":8080", "HTTP服务监听地址")
	flag.Parse()

	// 初始化存储引擎
	storageEngine, err := storage.Open("./data")
	if err != nil {
		log.Fatalf("存储引擎初始化失败: %v", err)
	}
	defer func() {
		if err := storageEngine.Close(); err != nil {
			log.Printf("存储引擎关闭失败: %v", err)
		}
	}()

	// TODO: 启动HTTP服务

	// 启动HTTP服务
	http.HandleFunc("/put", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		log.Printf("收到PUT请求 | 路径: %s | 参数key: %s", r.URL.Path, key)
		value, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "读取请求体失败", http.StatusBadRequest)
			return
		}

		log.Printf("开始存储数据 | key: %s | 数据大小: %d字节", key, len(value))
		if err := storageEngine.Put([]byte(key), value); err != nil {
			log.Printf("存储失败 | key: %s | 错误详情: %+v", key, err)
			w.Header().Set("Content-Type", "application/json")
			if len(value) == 0 {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"status":  "error",
					"message": "数据已删除",
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "success",
				"key":    key,
				"data":   string(value),
			})
			http.Error(w, "存储失败", http.StatusInternalServerError)
			return
		}
		log.Printf("数据存储成功 | key: %s | 数据大小: %d字节", key, len(value))
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "success",
			"key":       key,
			"data_size": len(value),
		})
	})

	http.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		log.Printf("收到GET请求 | key: %s", key)
		value, err := storageEngine.Get([]byte(key))
		if err != nil {
			log.Printf("数据查询失败 | key: %s | 错误: %+v", key, err)
			http.Error(w, "数据不存在", http.StatusNotFound)
			return
		}
		w.Write(value)
	})

	http.HandleFunc("/delete", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		log.Printf("收到DELETE请求 | key: %s", key)
		err := storageEngine.Delete([]byte(key))
		if err != nil {
			log.Printf("删除操作失败 | key: %s | 错误详情: %+v", key, err)
			http.Error(w, fmt.Sprintf("删除失败: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("删除成功"))
	})

	// 添加历史查询端点
	http.HandleFunc("/history", func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("start")
		end := r.URL.Query().Get("end")

		startTs, _ := strconv.ParseUint(start, 10, 64)
		endTs, _ := strconv.ParseUint(end, 10, 64)

		log.Printf("历史查询请求 | 时间范围: %d-%d", startTs, endTs)
		keys, err := storageEngine.TimeRangeQuery(startTs, endTs)
		if err != nil {
			log.Printf("时间范围查询失败 | 错误详情: %+v", err)
			http.Error(w, "查询失败", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "success",
			"message": "删除成功",
			"key":     keys,
		})
	})

	server := &http.Server{Addr: listenAddr}
	go func() {
		log.Printf("正在启动HTTP服务，监听地址: %s", listenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP服务启动失败: %+v", err)
		}
	}()

	// 等待退出信号
	// 创建信号通道
	sigCh := make(chan os.Signal, 1)
	<-sigCh
	log.Printf("LogAI Server 正在关闭...")

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("HTTP服务关闭失败: %v", err)
	}
	if err := storageEngine.Close(); err != nil {
		log.Printf("存储引擎关闭失败: %v", err)
	}
}
