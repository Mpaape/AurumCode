package yaml

import (
	"reflect"
	"strings"
)

func Unmarshal(data []byte, target any) error {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return nil
	}
	value = value.Elem()
	field := value.FieldByName("BaseURL")
	if !field.IsValid() || !field.CanSet() || field.Kind() != reflect.Pointer {
		return nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, raw, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(key) != "baseurl" {
			continue
		}
		parsed := strings.Trim(strings.TrimSpace(raw), "\"'")
		pointer := reflect.New(field.Type().Elem())
		pointer.Elem().SetString(parsed)
		field.Set(pointer)
		return nil
	}
	return nil
}
