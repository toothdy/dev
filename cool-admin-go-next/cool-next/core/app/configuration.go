package app

import (
	"bytes"
	"context"
	"io"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/configuration"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/module"
)

const (
	defaultConfigPath   = "manifest/config/config.yaml"
	configPathEnv       = "COOL_CONFIG_FILE"
	moduleConfiguration = "modules"
)

// 加载应用根配置与模块专属配置
func loadAssembleInput(ctx context.Context, definition Definition) (AssembleInput, error) {
	path := os.Getenv(configPathEnv)
	if path == "" {
		path = defaultConfigPath
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return AssembleInput{}, exception.WrapCore(err, "读取应用配置失败")
	}

	return parseAssembleInput(ctx, content, definition.graph.Modules())
}

func parseAssembleInput(ctx context.Context, content []byte, modules []module.StaticModule) (AssembleInput, error) {
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
	root, moduleNodes, err := splitConfigDocument(document, modules)
	if err != nil {
		return AssembleInput{}, err
	}
	rootSource, err := encodeConfigNode(root)
	if err != nil {
		return AssembleInput{}, err
	}
	moduleSources := make(map[string]configuration.Source, len(moduleNodes))
	for key, node := range moduleNodes {
		main, encodeErr := encodeConfigNode(node)
		if encodeErr != nil {
			return AssembleInput{}, encodeErr
		}
		moduleSources[key] = configuration.Source{Main: main, LookupEnv: os.LookupEnv}
	}

	return newAssembleInput(
		configuration.Source{Main: rootSource, LookupEnv: os.LookupEnv},
		moduleSources,
	), nil
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

func emptyConfigDocument() *yaml.Node {
	return &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
		}},
	}
}

func splitConfigDocument(document *yaml.Node, modules []module.StaticModule) (*yaml.Node, map[string]*yaml.Node, error) {
	root := document.Content[0]
	knownModules := make(map[string]bool, len(modules))
	for _, current := range modules {
		knownModules[current.Identity().Key()] = true
	}
	rootCopy := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	moduleNodes := make(map[string]*yaml.Node)
	seenRoot := make(map[string]bool, len(root.Content)/2)
	for index := 0; index < len(root.Content); index += 2 {
		key := root.Content[index]
		value := root.Content[index+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return nil, nil, exception.Core("应用配置顶层键必须是字符串")
		}
		if seenRoot[key.Value] {
			return nil, nil, exception.Core("应用配置顶层键重复: " + key.Value)
		}
		seenRoot[key.Value] = true
		if key.Value != moduleConfiguration {
			rootCopy.Content = append(rootCopy.Content, key, value)
			continue
		}
		if err := collectModuleNodes(value, knownModules, moduleNodes); err != nil {
			return nil, nil, err
		}
	}

	return rootCopy, moduleNodes, nil
}

func collectModuleNodes(node *yaml.Node, known map[string]bool, result map[string]*yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return exception.Core("应用配置 modules 必须是对象")
	}
	for index := 0; index < len(node.Content); index += 2 {
		key := node.Content[index]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return exception.Core("模块配置键必须是字符串")
		}
		if result[key.Value] != nil {
			return exception.Core("模块配置重复: " + key.Value)
		}
		if !known[key.Value] {
			return exception.Core("模块配置不存在对应模块: " + key.Value)
		}
		result[key.Value] = node.Content[index+1]
	}

	return nil
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
