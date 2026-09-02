package app

import (
	"bytes"
	"context"
	"io"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/config"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/module"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/route"
)

const (
	defaultConfigPath = "manifest/config/config.yaml"
	configPathEnv     = "COOL_CONFIG_FILE"
)

// 装配函数的只读配置视图
type AssembleInput struct {
	root config.Source
}

type assembleInputKey struct{}

func withInput(ctx context.Context, input AssembleInput) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, assembleInputKey{}, input)
}

func newInput(root config.Source) AssembleInput {
	return AssembleInput{root: cloneSource(root)}
}

// 根配置来源副本
func (input AssembleInput) RootSource() config.Source {
	return cloneSource(input.root)
}

// 模块默认配置来源
func (input AssembleInput) ModuleDefaultsSource() config.Source {
	return config.Source{LookupEnv: input.root.LookupEnv}
}

// AssembleFunc 负责按静态拓扑构造一次应用实例
type AssembleFunc func(context.Context, AssembleInput) (*Assembly, error)

// 保存静态 Graph 和唯一生成装配函数
type Definition struct {
	graph    module.Graph
	assemble AssembleFunc
}

// 静态路由表
func (definition Definition) Routes() route.Table { return definition.graph.Routes() }

// 静态 Graph
func (definition Definition) Graph() module.Graph { return definition.graph }

// 不可变应用定义
func Define(graph module.Graph, assemble AssembleFunc) Definition {
	if !graph.IsValidated() {
		panic("app Definition requires a validated module Graph")
	}
	if assemble == nil {
		panic("app Definition requires an assemble function")
	}

	return Definition{graph: graph, assemble: assemble}
}

func cloneSource(source config.Source) config.Source {
	return config.Source{Main: append([]byte(nil), source.Main...), LookupEnv: source.LookupEnv}
}

// 加载应用根配置
func loadInput(ctx context.Context) (AssembleInput, error) {
	path := os.Getenv(configPathEnv)
	if path == "" {
		path = defaultConfigPath
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return AssembleInput{}, exception.WrapCore(err, "读取应用配置失败")
	}

	return parseInput(ctx, content)
}

func parseInput(ctx context.Context, content []byte) (AssembleInput, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return AssembleInput{}, err
	}
	document, err := decodeConfig(content)
	if err != nil {
		return AssembleInput{}, err
	}
	if hasModuleConfig(document.Content[0]) {
		return AssembleInput{}, exception.Core("应用配置不支持 modules 节点")
	}
	rootSource, err := encodeConfig(document.Content[0])
	if err != nil {
		return AssembleInput{}, err
	}

	return newInput(config.Source{Main: rootSource, LookupEnv: os.LookupEnv}), nil
}

func decodeConfig(content []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	document := &yaml.Node{}
	if err := decoder.Decode(document); err != nil {
		if err == io.EOF {
			return emptyConfig(), nil
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

func hasModuleConfig(root *yaml.Node) bool {
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

func emptyConfig() *yaml.Node {
	return &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
		}},
	}
}

func encodeConfig(node *yaml.Node) ([]byte, error) {
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
