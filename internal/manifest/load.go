package manifest

import (
	"fmt"
	"io"
	"os"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// Load decodes one or more multi-document Kubernetes YAML/JSON files.
func Load(paths []string) ([]*unstructured.Unstructured, error) {
	var objects []*unstructured.Unstructured
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", path, err)
		}

		decoder := yaml.NewYAMLOrJSONDecoder(file, 4096)
		for document := 1; ; document++ {
			var raw map[string]any
			err := decoder.Decode(&raw)
			if err == io.EOF {
				break
			}
			if err != nil {
				file.Close()
				return nil, fmt.Errorf("decode %s document %d: %w", path, document, err)
			}
			if len(raw) == 0 {
				continue
			}
			object := &unstructured.Unstructured{Object: raw}
			if object.GetAPIVersion() == "" || object.GetKind() == "" || object.GetName() == "" {
				file.Close()
				return nil, fmt.Errorf("%s document %d must have apiVersion, kind, and metadata.name", path, document)
			}
			objects = append(objects, object)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("close %s: %w", path, err)
		}
	}
	if len(objects) == 0 {
		return nil, fmt.Errorf("no Kubernetes objects found")
	}
	return objects, nil
}
