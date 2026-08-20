package task

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	MaxExpressionLength    = 64 * 1024
	MaxExpressionArguments = 32
	MaxArgumentLength      = 32 * 1024
)

// Expression 表示已校验的 Task 处理器调用表达式
type Expression struct {
	Key       string
	Arguments []json.RawMessage
}

// ParseExpression 解析受限的 service.method(JSON...) 表达式
func ParseExpression(value string) (Expression, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Expression{}, fmt.Errorf("任务处理器表达式不能为空")
	}
	if len(value) > MaxExpressionLength {
		return Expression{}, fmt.Errorf("任务处理器表达式不能超过 %d 字节", MaxExpressionLength)
	}
	openIndex := strings.IndexByte(value, '(')
	if openIndex <= 0 || !strings.HasSuffix(value, ")") {
		return Expression{}, fmt.Errorf("任务处理器表达式格式错误")
	}
	key := strings.TrimSpace(value[:openIndex])
	if !isHandlerName(key) {
		return Expression{}, fmt.Errorf("任务处理器名称格式错误")
	}
	argumentText := strings.TrimSpace(value[openIndex+1 : len(value)-1])
	if argumentText == "" {
		return Expression{Key: key, Arguments: []json.RawMessage{}}, nil
	}

	decoder := json.NewDecoder(strings.NewReader("[" + argumentText + "]"))
	decoder.UseNumber()
	var arguments []json.RawMessage
	if err := decoder.Decode(&arguments); err != nil {
		return Expression{}, fmt.Errorf("任务处理器参数必须是合法 JSON: %w", err)
	}
	if err := ensureExpressionEOF(decoder); err != nil {
		return Expression{}, err
	}
	if len(arguments) > MaxExpressionArguments {
		return Expression{}, fmt.Errorf("任务处理器参数不能超过 %d 个", MaxExpressionArguments)
	}
	for _, argument := range arguments {
		if len(bytes.TrimSpace(argument)) > MaxArgumentLength {
			return Expression{}, fmt.Errorf("单个任务处理器参数不能超过 %d 字节", MaxArgumentLength)
		}
	}
	return Expression{Key: key, Arguments: arguments}, nil
}

func ensureExpressionEOF(decoder *json.Decoder) error {
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("任务处理器表达式包含多余数据")
		}
		return fmt.Errorf("任务处理器表达式格式错误: %w", err)
	}
	return nil
}

func isHandlerName(value string) bool {
	parts := strings.Split(value, ".")
	return len(parts) == 2 && isHandlerIdentifier(parts[0]) && isHandlerIdentifier(parts[1])
}

func isHandlerIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		item := value[index]
		isLetter := item >= 'a' && item <= 'z' || item >= 'A' && item <= 'Z'
		isDigit := item >= '0' && item <= '9'
		if !isLetter && item != '_' && !(index > 0 && isDigit) {
			return false
		}
	}
	return true
}
