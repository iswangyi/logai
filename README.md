# LogAI - 分布式日志系统
## 开发环境要求
- Go 1.21或更高版本
- Kubernetes集群（用于部署agent）
- Linux系统或Kubernetes集群（用于部署server）

## 使用示例

### 存储日志数据
```bash
curl -X POST http://localhost:8080/put?key=test_key \
  -H "Content-Type: application/octet-stream" \
  --data-binary @/path/to/logfile.log
```

### 检索日志数据
```bash
curl -X GET http://localhost:8080/get?key=test_key
```

### 通过管道直接传输数据
```bash
echo "log content" | curl -X POST http://localhost:8080/put?key=stream_key --data-binary @-
```

## 项目结构
```
.
├── cmd/                    # 主程序入口
│   ├── agent/             # 日志采集agent
│   └── server/            # 日志存储服务端
├── pkg/                   # 公共包
│   ├── common/            # 通用工具和类型定义
│   ├── collector/         # 日志采集相关
│   └── storage/           # 日志存储相关
├── internal/              # 内部包
│   ├── agent/             # agent内部实现
│   └── server/            # server内部实现
└── go.mod                 # Go模块定义
```