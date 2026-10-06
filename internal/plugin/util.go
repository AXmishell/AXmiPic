package plugin

import (
	"fmt"
	"os"
)

// readFile 读取插件入口文件。
func readFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("plugin: read entry %s: %w", path, err)
	}
	return data, nil
}
