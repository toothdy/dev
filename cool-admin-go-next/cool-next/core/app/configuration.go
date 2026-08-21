package app

import (
	"bytes"
	"context"
	"io"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/configuration"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

const (
	defaultConfigPath = "manifest/config/config.yaml"
	configPathEnv     = "COOL_CONFIG_FILE"
)

// 加载应用根配置
func loadAssembleInput(ctx context.Context, definition Definition) (AssembleInput, error) {
	path := os.Getenv(configPathEnv)
	if path == "" {
		path = defaultConfigPath
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return AssembleInput{}, exception.WrapCore(err, "读取应用配置失败")
	}

	return parseAssembleInput(ctx, content)
}

func parseAssembleInput(ctx context.Context, content []byte) (AssembleInput, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return AssembleInput{}, err
	}
	document, err := decodeConfigDocument(content)
	if err != nil {
		return AssembleInput{}, err
	}
	if hasModuleConfiguration(document.Content[0]) {
		return AssembleInput{}, exception.Core("应用配置不支持 modules 节点")
	}
	rootSource, err := encodeConfigNode(document.Content[0])
	if err != nil {
		return AssembleInput{}, err
	}

	return newAssembleInput(configuration.Source{Main: rootSource, LookupEnv: os.LookupEnv}), nil
}

func decodeConfigDocument(content []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	document := &yaml.Node{}
	if err := decoder.Decode(document); err != nil {
		if err == io.EOF {
			return emptyConfigDocument(), nil
		}
		return nil, exception.WrapCore(err, "解析应用配置失败")
	}
	extra := &yaml.Node{}
	if err := decoder.Decode(extra); err != io.EOF {
		if err == nil {
			return nil, exception.Core("应用配置只能包含一个 YAML 文档")
		}
		return nil, exception.WrapCore(err, "解析应用配置失败")
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, exception.Core("应用配置根节点必须是对象")
	}

	return document, nil
}

func hasModuleConfiguration(root *yaml.Node) bool {
	if root == nil || root.Kind != yaml.MappingNode {
		return false
	}
	for index := 0; index+1 < len(root.Content); index += 2 {
		if root.Content[index].Value == "modules" {
			return true
		}
	}
	return false
}

func emptyConfigDocument() *yaml.Node {
	return &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
		}},
	}
}

func encodeConfigNode(node *yaml.Node) ([]byte, error) {
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(node); err != nil {
		return nil, exception.WrapCore(err, "编码应用配置失败")
	}
	if err := encoder.Close(); err != nil {
		return nil, exception.WrapCore(err, "编码应用配置失败")
	}

	return output.Bytes(), nil
}
