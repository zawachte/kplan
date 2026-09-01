package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

type Action string

const (
	Create    Action = "create"
	Update    Action = "update"
	Unchanged Action = "unchanged"
	Conflict  Action = "conflict"
)

type Change struct {
	Action    Action
	Group     string
	Kind      string
	Namespace string
	Name      string
	Diff      string
	Detail    string
}

type Engine struct {
	dynamic          dynamic.Interface
	mapper           meta.RESTMapper
	defaultNamespace string
	fieldManager     string
	forceConflicts   bool
}

func New(dynamicClient dynamic.Interface, mapper meta.RESTMapper, defaultNamespace, fieldManager string, forceConflicts bool) *Engine {
	return &Engine{dynamic: dynamicClient, mapper: mapper, defaultNamespace: defaultNamespace, fieldManager: fieldManager, forceConflicts: forceConflicts}
}

func (e *Engine) Plan(ctx context.Context, objects []*unstructured.Unstructured) ([]Change, error) {
	changes := make([]Change, 0, len(objects))
	for _, object := range objects {
		mapping, resource, err := e.resourceFor(object)
		if err != nil {
			return nil, err
		}
		live, err := resource.Get(ctx, object.GetName(), metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			changes = append(changes, changeFor(Create, mapping.GroupVersionKind, object))
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("get %s %s: %w", object.GetKind(), object.GetName(), err)
		}

		payload, err := json.Marshal(object.Object)
		if err != nil {
			return nil, fmt.Errorf("encode %s %s: %w", object.GetKind(), object.GetName(), err)
		}
		predicted, err := resource.Patch(ctx, object.GetName(), types.ApplyPatchType, payload, metav1.PatchOptions{
			DryRun:       []string{metav1.DryRunAll},
			FieldManager: e.fieldManager,
			Force:        ptr.To(e.forceConflicts),
		})
		if apierrors.IsConflict(err) {
			change := changeFor(Conflict, mapping.GroupVersionKind, object)
			change.Detail = err.Error()
			// This second request is still dry-run. Force is used only to obtain
			// the predicted representation needed for the conflict diff.
			preview, previewErr := resource.Patch(ctx, object.GetName(), types.ApplyPatchType, payload, metav1.PatchOptions{
				DryRun:       []string{metav1.DryRunAll},
				FieldManager: e.fieldManager,
				Force:        ptr.To(true),
			})
			if previewErr == nil {
				change.Diff, _ = objectDiff(live, preview)
			}
			changes = append(changes, change)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("dry-run apply %s %s: %w", object.GetKind(), object.GetName(), err)
		}
		action := Update
		if reflect.DeepEqual(normalize(live.Object), normalize(predicted.Object)) {
			action = Unchanged
		}
		change := changeFor(action, mapping.GroupVersionKind, object)
		if action == Update {
			change.Diff, err = objectDiff(live, predicted)
			if err != nil {
				return nil, fmt.Errorf("render diff for %s %s: %w", object.GetKind(), object.GetName(), err)
			}
		}
		changes = append(changes, change)
	}
	return changes, nil
}

func (e *Engine) Apply(ctx context.Context, objects []*unstructured.Unstructured) error {
	for _, object := range objects {
		_, resource, err := e.resourceFor(object)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(object.Object)
		if err != nil {
			return fmt.Errorf("encode %s %s: %w", object.GetKind(), object.GetName(), err)
		}
		if _, err := resource.Patch(ctx, object.GetName(), types.ApplyPatchType, payload, metav1.PatchOptions{
			FieldManager: e.fieldManager,
			Force:        ptr.To(e.forceConflicts),
		}); err != nil {
			return fmt.Errorf("apply %s %s: %w", object.GetKind(), object.GetName(), err)
		}
	}
	return nil
}

func (e *Engine) resourceFor(object *unstructured.Unstructured) (*meta.RESTMapping, dynamic.ResourceInterface, error) {
	gvk := object.GroupVersionKind()
	mapping, err := e.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, nil, fmt.Errorf("map %s: %w", gvk.String(), err)
	}
	var resource dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		namespace := object.GetNamespace()
		if namespace == "" {
			namespace = e.defaultNamespace
			object.SetNamespace(namespace)
		}
		resource = e.dynamic.Resource(mapping.Resource).Namespace(namespace)
	} else {
		resource = e.dynamic.Resource(mapping.Resource)
	}
	return mapping, resource, nil
}

func changeFor(action Action, gvk schema.GroupVersionKind, object *unstructured.Unstructured) Change {
	return Change{Action: action, Group: gvk.Group, Kind: gvk.Kind, Namespace: object.GetNamespace(), Name: object.GetName()}
}

func normalize(input map[string]any) map[string]any {
	copy := (&unstructured.Unstructured{Object: input}).DeepCopy().Object
	unstructured.RemoveNestedField(copy, "status")
	for _, field := range []string{"creationTimestamp", "generation", "managedFields", "resourceVersion", "uid"} {
		unstructured.RemoveNestedField(copy, "metadata", field)
	}
	if annotations, found, _ := unstructured.NestedStringMap(copy, "metadata", "annotations"); found {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		if len(annotations) == 0 {
			unstructured.RemoveNestedField(copy, "metadata", "annotations")
		} else {
			_ = unstructured.SetNestedStringMap(copy, annotations, "metadata", "annotations")
		}
	}
	return copy
}

func objectDiff(live, predicted *unstructured.Unstructured) (string, error) {
	liveYAML, err := yaml.Marshal(normalize(live.Object))
	if err != nil {
		return "", err
	}
	plannedYAML, err := yaml.Marshal(normalize(predicted.Object))
	if err != nil {
		return "", err
	}
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(liveYAML)),
		B:        difflib.SplitLines(string(plannedYAML)),
		FromFile: "live",
		ToFile:   "planned",
		Context:  3,
	})
	return strings.TrimSpace(diff), err
}

func Summary(changes []Change) map[Action]int {
	result := map[Action]int{Create: 0, Update: 0, Unchanged: 0, Conflict: 0}
	for _, change := range changes {
		result[change.Action]++
	}
	return result
}

func SortForApply(objects []*unstructured.Unstructured) {
	sort.SliceStable(objects, func(i, j int) bool {
		return objects[i].GetKind() == "Namespace" && objects[j].GetKind() != "Namespace"
	})
}
