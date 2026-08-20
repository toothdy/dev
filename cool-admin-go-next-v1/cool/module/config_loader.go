package module

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

var durationType = reflect.TypeFor[time.Duration]()

// LoadConfig 从 module.<key> 加载并校验强类型模块配置。
func LoadConfig[T any](ctx context.Context, key string, defaults T) (T, error) {
	var zero T
	path := "module." + key
	loaded := deepCopyValue(reflect.ValueOf(defaults))
	if !loaded.IsValid() {
		return zero, fmt.Errorf("加载模块 %s 配置 %s 失败: 默认值无效", key, path)
	}

	moduleValue, err := g.Cfg().Get(ctx, "module")
	if err != nil {
		return zero, fmt.Errorf("加载模块 %s 配置 %s 失败: %w", key, path, err)
	}
	if moduleValue != nil {
		moduleMap, ok := stringMap(moduleValue.Interface())
		if !ok {
			return zero, fmt.Errorf("加载模块 %s 配置 %s 失败: module 必须是对象", key, path)
		}
		raw, exists := moduleMap[key]
		if exists {
			if raw == nil {
				return zero, fmt.Errorf("加载模块 %s 配置 %s 失败: 配置根不能为 null", key, path)
			}
			if _, ok = stringMap(raw); !ok {
				return zero, fmt.Errorf("加载模块 %s 配置 %s 失败: 配置根必须是对象", key, path)
			}
			if err = overlayValue(loaded, raw, path); err != nil {
				return zero, fmt.Errorf("加载模块 %s 配置失败: %w", key, err)
			}
		}
	}

	result, ok := loaded.Interface().(T)
	if !ok {
		return zero, fmt.Errorf("加载模块 %s 配置 %s 失败: 结果类型不匹配", key, path)
	}
	if validator, validatable := any(result).(interface{ Validate() error }); validatable {
		if err = validator.Validate(); err != nil {
			return zero, fmt.Errorf("加载模块 %s 配置 %s 校验失败: %w", key, path, err)
		}
	}
	return result, nil
}

func deepCopyValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return reflect.Value{}
	}
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		copied := reflect.New(value.Type().Elem())
		copied.Elem().Set(deepCopyValue(value.Elem()))
		return copied
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		copied := deepCopyValue(value.Elem())
		result := reflect.New(value.Type()).Elem()
		result.Set(copied)
		return result
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		copied := reflect.MakeSlice(value.Type(), value.Len(), value.Cap())
		for index := 0; index < value.Len(); index++ {
			copied.Index(index).Set(deepCopyValue(value.Index(index)))
		}
		return copied
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		copied := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			copied.SetMapIndex(deepCopyValue(iterator.Key()), deepCopyValue(iterator.Value()))
		}
		return copied
	case reflect.Struct:
		copied := reflect.New(value.Type()).Elem()
		copied.Set(value)
		for index := 0; index < value.NumField(); index++ {
			if copied.Field(index).CanSet() && value.Type().Field(index).IsExported() {
				copied.Field(index).Set(deepCopyValue(value.Field(index)))
			}
		}
		return copied
	default:
		return value
	}
}

func overlayValue(target reflect.Value, raw any, path string) error {
	if raw == nil {
		switch target.Kind() {
		case reflect.Pointer, reflect.Map, reflect.Slice:
			target.SetZero()
			return nil
		default:
			return fmt.Errorf("%s 不能为 null", path)
		}
	}

	if target.Type() == durationType {
		value, ok := raw.(string)
		if !ok {
			return fmt.Errorf("%s 必须是 duration 字符串", path)
		}
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("%s 解析 duration 失败: %w", path, err)
		}
		target.SetInt(int64(duration))
		return nil
	}

	switch target.Kind() {
	case reflect.Pointer:
		if target.IsNil() {
			target.Set(reflect.New(target.Type().Elem()))
		}
		return overlayValue(target.Elem(), raw, path)
	case reflect.Struct:
		return overlayStruct(target, raw, path)
	case reflect.Slice:
		values, ok := sliceValue(raw)
		if !ok {
			return fmt.Errorf("%s 必须是数组", path)
		}
		result := reflect.MakeSlice(target.Type(), len(values), len(values))
		for index, value := range values {
			if err := overlayValue(result.Index(index), value, fmt.Sprintf("%s.%d", path, index)); err != nil {
				return err
			}
		}
		target.Set(result)
		return nil
	case reflect.Map:
		return overlayMap(target, raw, path)
	case reflect.String:
		value, ok := raw.(string)
		if !ok {
			return fmt.Errorf("%s 必须是字符串", path)
		}
		target.SetString(value)
		return nil
	case reflect.Bool:
		value, ok := raw.(bool)
		if !ok {
			return fmt.Errorf("%s 必须是布尔值", path)
		}
		target.SetBool(value)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, ok := signedInteger(raw)
		if !ok || target.OverflowInt(value) {
			return fmt.Errorf("%s 必须是整数", path)
		}
		target.SetInt(value)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value, ok := unsignedInteger(raw)
		if !ok || target.OverflowUint(value) {
			return fmt.Errorf("%s 必须是非负整数", path)
		}
		target.SetUint(value)
		return nil
	case reflect.Float32, reflect.Float64:
		value, ok := floatingPoint(raw)
		if !ok || target.OverflowFloat(value) {
			return fmt.Errorf("%s 必须是数字", path)
		}
		target.SetFloat(value)
		return nil
	default:
		return fmt.Errorf("%s 使用了不支持的配置类型 %s", path, target.Type())
	}
}

func overlayStruct(target reflect.Value, raw any, path string) error {
	values, ok := stringMap(raw)
	if !ok {
		return fmt.Errorf("%s 必须是对象", path)
	}
	fields := make(map[string]int, target.NumField())
	for index := 0; index < target.NumField(); index++ {
		field := target.Type().Field(index)
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			fields[name] = index
		}
	}
	for name, value := range values {
		index, exists := fields[name]
		if !exists {
			return fmt.Errorf("%s.%s 是未知字段", path, name)
		}
		if err := overlayValue(target.Field(index), value, path+"."+name); err != nil {
			return err
		}
	}
	return nil
}

func overlayMap(target reflect.Value, raw any, path string) error {
	values, ok := stringMap(raw)
	if !ok || target.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("%s 必须是字符串键对象", path)
	}
	result := reflect.MakeMapWithSize(target.Type(), len(values))
	for name, value := range values {
		mapValue := reflect.New(target.Type().Elem()).Elem()
		if err := overlayValue(mapValue, value, path+"."+name); err != nil {
			return err
		}
		key := reflect.New(target.Type().Key()).Elem()
		key.SetString(name)
		result.SetMapIndex(key, mapValue)
	}
	target.Set(result)
	return nil
}

func stringMap(value any) (map[string]any, bool) {
	if typed, ok := value.(map[string]any); ok {
		return typed, true
	}
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || reflected.Kind() != reflect.Map {
		return nil, false
	}
	result := make(map[string]any, reflected.Len())
	iterator := reflected.MapRange()
	for iterator.Next() {
		if iterator.Key().Kind() != reflect.String {
			return nil, false
		}
		result[iterator.Key().String()] = iterator.Value().Interface()
	}
	return result, true
}

func sliceValue(value any) ([]any, bool) {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || (reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array) {
		return nil, false
	}
	result := make([]any, reflected.Len())
	for index := range result {
		result[index] = reflected.Index(index).Interface()
	}
	return result, true
}

func signedInteger(value any) (int64, bool) {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return 0, false
	}
	switch reflected.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflected.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if reflected.Uint() > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(reflected.Uint()), true
	default:
		return 0, false
	}
}

func unsignedInteger(value any) (uint64, bool) {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return 0, false
	}
	switch reflected.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if reflected.Int() < 0 {
			return 0, false
		}
		return uint64(reflected.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return reflected.Uint(), true
	default:
		return 0, false
	}
}

func floatingPoint(value any) (float64, bool) {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return 0, false
	}
	switch reflected.Kind() {
	case reflect.Float32, reflect.Float64:
		return reflected.Float(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(reflected.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(reflected.Uint()), true
	default:
		return 0, false
	}
}
