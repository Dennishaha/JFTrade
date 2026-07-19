package frontendassets

import (
	"io/fs"
	"log"
)

// Load 返回可用的前端资源 FS；资源未嵌入或解压失败时返回 nil。
// 这是 apiserver 与 servercore 共用的统一加载策略。
func Load() fs.FS {
	return LoadWith(FileSystem)
}

// LoadWith 把嵌入资源失败策略从 build-tag 选中的资源提供者中分离出来，
// 便于测试独立覆盖。
func LoadWith(loader func() (fs.FS, bool, error)) fs.FS {
	frontendFS, available, err := loader()
	if err != nil {
		log.Printf("JFTrade embedded frontend assets unavailable: %v", err)
		return nil
	}
	if !available {
		return nil
	}
	return frontendFS
}
