package service_test

import (
	"reflect"
	"testing"

	"github.com/toothdy/cool-admin-go-next/modules/task/service"
)

func TestParseInvocation(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		target    string
		arguments []any
	}{
		{name: "无参", input: "taskDemoService.test()", target: "taskDemoService.test", arguments: []any{}},
		{name: "多个数字参数", input: "taskDemoService.test(1,2)", target: "taskDemoService.test",
			arguments: []any{float64(1), float64(2)}},
		{name: "参数间空白", input: "taskDemoService.test( 1 , 2 )", target: "taskDemoService.test",
			arguments: []any{float64(1), float64(2)}},
		{name: "数组参数不被逗号切开", input: "taskDemoService.test([1, 2])", target: "taskDemoService.test",
			arguments: []any{[]any{float64(1), float64(2)}}},
		{name: "对象参数", input: `taskDemoService.test({"a":1,"b":2})`, target: "taskDemoService.test",
			arguments: []any{map[string]any{"a": float64(1), "b": float64(2)}}},
		{name: "非 JSON 参数按原始字符串传入", input: "taskDemoService.test(abc,1)", target: "taskDemoService.test",
			arguments: []any{"abc", float64(1)}},
		{name: "带引号字符串内的逗号保留", input: `taskDemoService.test("a,b")`, target: "taskDemoService.test",
			arguments: []any{"a,b"}},
	}
	for _, current := range cases {
		t.Run(current.name, func(t *testing.T) {
			target, arguments, err := service.ParseInvocation(current.input)
			if err != nil {
				t.Fatalf("解析调用字符串失败: %v", err)
			}
			if target != current.target {
				t.Errorf("目标 = %q，期望 %q", target, current.target)
			}
			if !reflect.DeepEqual(arguments, current.arguments) {
				t.Errorf("参数 = %#v，期望 %#v", arguments, current.arguments)
			}
		})
	}
}

func TestParseInvocationRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{
		"", "taskDemoService", "taskDemoService.test", "taskDemoService.test(", ".test()",
		"taskDemoService.()", "taskDemoService.a.b()", "taskDemoService.test([1,2)", "taskDemoService.test(1,)",
	} {
		if _, _, err := service.ParseInvocation(input); err == nil {
			t.Errorf("%q 期望解析失败", input)
		}
	}
}

func TestRegistryInvoke(t *testing.T) {
	demo, err := service.NewDemo()
	if err != nil {
		t.Fatalf("构造演示任务失败: %v", err)
	}
	registry, err := service.NewRegistry(demo)
	if err != nil {
		t.Fatalf("构造调用注册表失败: %v", err)
	}
	result, err := registry.Invoke(t.Context(), "taskDemoService.test(1,2)")
	if err != nil {
		t.Fatalf("调用任务目标失败: %v", err)
	}
	if result != "任务执行成功" {
		t.Errorf("返回值 = %v，期望 任务执行成功", result)
	}
	if _, err = registry.Invoke(t.Context(), "missingService.test()"); err == nil {
		t.Error("未注册目标期望调用失败")
	}
}
