package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// StorageEngine 存储引擎接口
type StorageEngine interface {
	Put(key []byte, value []byte) error
	Get(key []byte) ([]byte, error)
	Delete(key []byte) error
	TimeRangeQuery(startTs, endTs uint64) ([]string, error)
	Close() error
}

// Bitcask 存储引擎实现
// 在Bitcask结构体中添加压缩相关字段
type TimeIndexEntry struct {
	timestamp uint64
	key       string
	offset    int64
}

type Bitcask struct {
	dataDir            string
	activeFile         *os.File
	index              map[string]int64
	timeIndex          []TimeIndexEntry
	mu                 sync.RWMutex
	compactionInterval time.Duration
	stopCompaction     chan struct{}
	segmentInterval    time.Duration
	lastSegmentTime    time.Time
}

// 在Put方法中记录时间索引
func (b *Bitcask) Put(key []byte, value []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 检查时间分段条件
	if time.Since(b.lastSegmentTime) > b.segmentInterval {
		if err := b.rollActiveFile(); err != nil {
			return err
		}
	}

	// 序列化数据：时间戳(8字节)+键长(4字节)+值长(4字节)+键+值
	ts := uint64(time.Now().UnixNano())
	keySize := uint32(len(key))
	valSize := uint32(len(value))

	buf := make([]byte, 16+keySize+valSize)
	binary.BigEndian.PutUint64(buf[0:8], ts)
	binary.BigEndian.PutUint32(buf[8:12], keySize)
	binary.BigEndian.PutUint32(buf[12:16], valSize)
	copy(buf[16:16+keySize], key)
	copy(buf[16+keySize:], value)

	offset, err := b.activeFile.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("文件定位失败: %w", err)
	}

	if _, err := b.activeFile.Write(buf); err != nil {
		return fmt.Errorf("数据写入失败: %w", err)
	}

	// 更新内存索引
	b.index[string(key)] = offset

	// 更新时间索引
	entry := TimeIndexEntry{
		timestamp: ts,
		key:       string(key),
		offset:    offset,
	}
	b.timeIndex = append(b.timeIndex, entry)

	// 保持时间索引有序
	sort.Slice(b.timeIndex, func(i, j int) bool {
		return b.timeIndex[i].timestamp < b.timeIndex[j].timestamp
	})

	// 检查文件大小是否需要滚动(1MB阈值)
	if stat, _ := b.activeFile.Stat(); stat.Size() > 1<<20 {
		if err := b.rollActiveFile(); err != nil {
			return err
		}
	}

	return nil
}

func (b *Bitcask) Get(key []byte) ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	offset, exists := b.index[string(key)]
	if !exists {
		return nil, errors.New("key not found")
	}

	file := b.activeFile
	// 如果偏移量不在当前活跃文件，需要查找历史文件
	// 这里暂时只处理当前活跃文件的情况

	_, err := file.Seek(offset, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("文件定位失败: %w", err)
	}

	header := make([]byte, 16)
	if _, err := io.ReadFull(file, header); err != nil {
		return nil, fmt.Errorf("读取头部失败: %w", err)
	}

	keySize := binary.BigEndian.Uint32(header[8:12])
	valSize := binary.BigEndian.Uint32(header[12:16])

	// 检查墓碑记录
	if valSize == 0 {
		return nil, errors.New("key deleted")
	}

	data := make([]byte, keySize+valSize)
	if _, err := io.ReadFull(file, data); err != nil {
		return nil, fmt.Errorf("读取数据失败: %w", err)
	}

	storedKey := data[:keySize]
	if string(storedKey) != string(key) {
		return nil, errors.New("key mismatch")
	}

	return data[keySize:], nil
}

func (b *Bitcask) Delete(key []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 写入墓碑记录（0长度值）
	if err := b.Put(key, []byte{}); err != nil {
		return err
	}

	// 更新内存索引（标记为已删除）
	b.index[string(key)] = -1
	return nil
}

// 新增时间范围查询方法
func (b *Bitcask) TimeRangeQuery(startTs, endTs uint64) ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var keys []string
	for _, entry := range b.timeIndex {
		if entry.timestamp >= startTs && entry.timestamp <= endTs {
			keys = append(keys, entry.key)
		}
	}
	return keys, nil
}

func (b *Bitcask) Close() error {
	return b.activeFile.Close()
}

// 实现文件滚动方法
func (b *Bitcask) rollActiveFile() error {
	// 创建新文件
	newFile, err := os.Create(filepath.Join(b.dataDir, "active_new.data"))
	if err != nil {
		return fmt.Errorf("创建新活跃文件失败: %w", err)
	}

	// 原子替换操作
	oldFile := b.activeFile
	b.activeFile = newFile

	// 关闭旧文件
	if err := oldFile.Close(); err != nil {
		return fmt.Errorf("关闭旧文件失败: %w", err)
	}

	// 重命名旧文件
	timestamp := time.Now().Unix()
	oldPath := filepath.Join(b.dataDir, oldFile.Name())
	newPath := filepath.Join(b.dataDir, fmt.Sprintf("segment-%d.data", timestamp))
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("文件重命名失败: %w", err)
	}

	return nil
}

// Open函数实现
func Open(dataDir string) (StorageEngine, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	activeFile, err := os.OpenFile(
		filepath.Join(dataDir, "active.data"),
		os.O_CREATE|os.O_RDWR,
		0644,
	)
	if err != nil {
		return nil, fmt.Errorf("打开活跃文件失败: %w", err)
	}

	b := &Bitcask{
		dataDir:            dataDir,
		activeFile:         activeFile,
		index:              make(map[string]int64),
		timeIndex:          make([]TimeIndexEntry, 0),
		compactionInterval: time.Hour * 24,
		stopCompaction:     make(chan struct{}),
		segmentInterval:    time.Hour,
		lastSegmentTime:    time.Now(),
	}

	go b.startCompaction()
	return b, nil
}

// 启动定期压缩任务
func (b *Bitcask) startCompaction() {
	ticker := time.NewTicker(b.compactionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := b.compress(); err != nil {
				log.Printf("压缩失败: %v", err)
			}
		case <-b.stopCompaction:
			return
		}
	}
}

// 压缩核心逻辑
func (b *Bitcask) compress() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 获取所有历史文件
	files, err := filepath.Glob(filepath.Join(b.dataDir, "*.data"))
	if err != nil {
		return fmt.Errorf("查找数据文件失败: %w", err)
	}

	// 过滤活跃文件
	var oldFiles []string
	for _, f := range files {
		if filepath.Base(f) != "active.data" {
			oldFiles = append(oldFiles, f)
		}
	}

	// 创建临时压缩文件
	compactedFile, err := os.CreateTemp(b.dataDir, "compact-*.data")
	if err != nil {
		return fmt.Errorf("创建压缩文件失败: %w", err)
	}
	defer os.Remove(compactedFile.Name())

	// 合并有效数据
	newIndex := make(map[string]int64)
	for _, f := range oldFiles {
		file, err := os.Open(f)
		if err != nil {
			return fmt.Errorf("打开文件%s失败: %w", f, err)
		}

		offset := int64(0)
		for {
			header := make([]byte, 16)
			_, err := io.ReadFull(file, header)
			if err == io.EOF {
				break
			}

			keySize := binary.BigEndian.Uint32(header[8:12])
			valSize := binary.BigEndian.Uint32(header[12:16])
			data := make([]byte, keySize+valSize)
			io.ReadFull(file, data)

			key := string(data[:keySize])
			if valSize > 0 && b.index[key] != -1 {
				if _, err := compactedFile.Write(append(header, data...)); err != nil {
					return err
				}
				newIndex[key] = offset
				offset += int64(16 + keySize + valSize)
			}
		}
		file.Close()
	}

	// 原子替换索引和文件
	for key, offset := range newIndex {
		b.index[key] = offset
	}
	for _, f := range oldFiles {
		if err := os.Remove(f); err != nil {
			return fmt.Errorf("删除旧文件失败: %w", err)
		}
	}

	return nil
}
