package app

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

const (
	jwtSecretPlaceholder = "cool-admin-go-next-xxxxxx"
	defaultConfigPath    = "manifest/config/config.yaml"
)

type jwtSecretGenerator func() (string, error)
type jwtConfigWriter func(path string, content []byte, mode fs.FileMode) error

func bootstrapJWTSecret(configPath string, effectiveSecret string) (string, error) {
	return bootstrapJWTSecretWithDeps(configPath, effectiveSecret, generateJWTSecret, writeJWTConfig)
}

func bootstrapJWTSecretWithDeps(
	configPath string,
	effectiveSecret string,
	generate jwtSecretGenerator,
	write jwtConfigWriter,
) (string, error) {
	if effectiveSecret != jwtSecretPlaceholder {
		return effectiveSecret, nil
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("读取 JWT 配置文件失败: %w", err)
	}
	info, err := os.Stat(configPath)
	if err != nil {
		return "", fmt.Errorf("读取 JWT 配置文件属性失败: %w", err)
	}

	document, err := decodeJWTConfig(content)
	if err != nil {
		return "", err
	}
	secretNode, err := findYAMLScalar(document, "cool", "auth", "jwtSecret")
	if err != nil {
		return "", err
	}
	if secretNode.Value != jwtSecretPlaceholder {
		return "", fmt.Errorf("JWT 配置文件中的占位符已发生变化")
	}

	secret, err := generate()
	if err != nil {
		return "", fmt.Errorf("生成 JWT 密钥失败: %w", err)
	}
	secretNode.SetString(secret)

	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err = encoder.Encode(document); err != nil {
		return "", fmt.Errorf("编码 JWT 配置文件失败: %w", err)
	}
	if err = encoder.Close(); err != nil {
		return "", fmt.Errorf("结束编码 JWT 配置文件失败: %w", err)
	}
	if err = write(configPath, output.Bytes(), info.Mode().Perm()); err != nil {
		return "", fmt.Errorf("写回 JWT 配置文件失败: %w", err)
	}
	return secret, nil
}

func generateJWTSecret() (string, error) {
	value, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return value.String(), nil
}

func decodeJWTConfig(content []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("解析 JWT 配置文件失败: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("JWT 配置文件只能包含一个 YAML 文档")
		}
		return nil, fmt.Errorf("解析 JWT 配置文件失败: %w", err)
	}
	return &document, nil
}

func findYAMLScalar(document *yaml.Node, path ...string) (*yaml.Node, error) {
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, fmt.Errorf("JWT 配置文件根节点无效")
	}
	current := document.Content[0]
	for index, part := range path {
		if current.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("JWT 配置节点 %q 不是对象", part)
		}
		var next *yaml.Node
		for child := 0; child+1 < len(current.Content); child += 2 {
			if current.Content[child].Value == part {
				next = current.Content[child+1]
				break
			}
		}
		if next == nil {
			return nil, fmt.Errorf("JWT 配置节点 %q 不存在", part)
		}
		if index == len(path)-1 {
			if next.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("JWT 配置目标节点不是标量")
			}
			return next, nil
		}
		current = next
	}
	return nil, fmt.Errorf("JWT 配置路径不能为空")
}

func writeJWTConfig(path string, content []byte, mode fs.FileMode) error {
	return atomicWriteJWTConfig(path, content, mode, os.Rename)
}

func atomicWriteJWTConfig(
	path string,
	content []byte,
	mode fs.FileMode,
	rename func(oldPath string, newPath string) error,
) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".jwt-config-*")
	if err != nil {
		return fmt.Errorf("创建临时配置文件失败: %w", err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
	}()

	if _, err = temporary.Write(content); err != nil {
		return fmt.Errorf("写入临时配置文件失败: %w", err)
	}
	if err = temporary.Chmod(mode.Perm()); err != nil {
		return fmt.Errorf("设置临时配置文件权限失败: %w", err)
	}
	if err = temporary.Sync(); err != nil {
		return fmt.Errorf("同步临时配置文件失败: %w", err)
	}
	if err = temporary.Close(); err != nil {
		return fmt.Errorf("关闭临时配置文件失败: %w", err)
	}
	closed = true
	if err = rename(temporaryPath, path); err != nil {
		return fmt.Errorf("替换 JWT 配置文件失败: %w", err)
	}

	directoryHandle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("打开配置目录失败: %w", err)
	}
	defer directoryHandle.Close()
	if err = directoryHandle.Sync(); err != nil {
		return fmt.Errorf("同步配置目录失败: %w", err)
	}
	return nil
}
