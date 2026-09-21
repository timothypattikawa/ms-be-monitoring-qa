package repository

import (
	"reflect"
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
		seen[typ] = true
	}
}
