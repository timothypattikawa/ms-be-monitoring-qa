package repository

import (
	"reflect"
	"strings"
	"testing"
)

func TestMonitoringModelInventory(t *testing.T) {
	models := Models()
	if len(models) != 12 {
		t.Fatalf("got %d monitoring models, want 12", len(models))
	}
	seen := map[reflect.Type]bool{}
	for _, model := range models {
		typ := reflect.TypeOf(model)
		if typ.Kind() != reflect.Pointer || seen[typ] {
			t.Fatalf("model inventory contains invalid or duplicate type %v", typ)
		}
		id, ok := typ.Elem().FieldByName("ID")
		if !ok || strings.Contains(id.Tag.Get("gorm"), "type:uuid") {
			t.Fatalf("%v must keep string IDs compatible with relation columns", typ)
		}
		seen[typ] = true
	}
}
