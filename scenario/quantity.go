package scenario

import (
	"fmt"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/resource"
)

type Quantity struct {
	resource.Quantity
}

func (q *Quantity) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("expected a scalar, got %s", value)
	}

	parsed, err := resource.ParseQuantity(value.Value)
	if err != nil {
		return fmt.Errorf("parsing quantity, %w", err)
	}
	q.Quantity = parsed
	return nil
}

func (q Quantity) String() string {
	return q.Quantity.String()
}
