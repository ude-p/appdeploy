package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ude-p/appdeploy/internal/config"
)

func TestSchemaTopicsFollowModel(t *testing.T) {
	modelType := reflect.TypeFor[config.Model]()
	want := make([]string, 0, modelType.NumField()+1)
	for field := range modelType.Fields() {
		tag := field.Tag.Get("hcl")
		if tag == "" {
			continue
		}
		want = append(want, strings.Split(tag, ",")[0])
	}
	want = append(want, "scope")

	if len(schemaTopics) != len(want) {
		t.Fatalf("got %d schema topics, want %d", len(schemaTopics), len(want))
	}
	for i, name := range want {
		if schemaTopics[i].name != name {
			t.Fatalf("schema topic %d = %q, want %q", i, schemaTopics[i].name, name)
		}
		if strings.TrimSpace(schemaTopics[i].summary) == "" {
			t.Errorf("schema topic %q has no summary", name)
		}
		if strings.TrimSpace(schemaTopics[i].body) == "" {
			t.Errorf("schema topic %q has no body", name)
		}
	}
}

func TestFindSchemaTopic(t *testing.T) {
	if _, ok := findSchemaTopic("app"); !ok {
		t.Fatal("app topic not found")
	}
	if _, ok := findSchemaTopic("nope"); ok {
		t.Fatal("unknown topic found")
	}
}
