package engine

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNormalizeRemovesServerFields(t *testing.T) {
	object := map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":            "example",
			"resourceVersion": "42",
			"uid":             "abc",
		},
		"data":   map[string]any{"hello": "world"},
		"status": map[string]any{"ignored": true},
	}
	got := normalize(object)
	if _, found, _ := unstructured.NestedFieldNoCopy(got, "metadata", "resourceVersion"); found {
		t.Fatal("resourceVersion was not removed")
	}
	if _, found := got["status"]; found {
		t.Fatal("status was not removed")
	}
	if _, found, _ := unstructured.NestedString(got, "data", "hello"); !found {
		t.Fatal("desired data was removed")
	}
}

func TestObjectDiffShowsDesiredChangeAndOmitsServerFields(t *testing.T) {
	live := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":            "example",
			"resourceVersion": "42",
		},
		"spec": map[string]any{"replicas": int64(2)},
	}}
	planned := live.DeepCopy()
	planned.SetResourceVersion("43")
	if err := unstructured.SetNestedField(planned.Object, int64(3), "spec", "replicas"); err != nil {
		t.Fatal(err)
	}

	diff, err := objectDiff(live, planned)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--- live", "+++ planned", "-  replicas: 2", "+  replicas: 3"} {
		if !strings.Contains(diff, want) {
			t.Fatalf("diff does not contain %q:\n%s", want, diff)
		}
	}
	if strings.Contains(diff, "resourceVersion") {
		t.Fatalf("diff contains server-managed resourceVersion:\n%s", diff)
	}
}
