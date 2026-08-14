package proxy

import (
	"gopkg.in/yaml.v3"
)

// ExtractRaw returns the YAML bytes of the top-level "proxy" mapping from a full agent.yaml.
// Missing or non-mapping proxy returns nil (plugin stays off). Parse errors return err so
// callers can soft-fail without blocking core config - prefer logging and nil.
func ExtractRaw(doc []byte) ([]byte, error) {
	if len(doc) == 0 {
		return nil, nil
	}
	var root yaml.Node
	if err := yaml.Unmarshal(doc, &root); err != nil {
		return nil, err
	}
	// Document node -> mapping
	node := &root
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k.Value != "proxy" {
			continue
		}
		if v.Kind != yaml.MappingNode {
			return nil, nil
		}
		b, err := yaml.Marshal(v)
		if err != nil {
			return nil, err
		}
		return b, nil
	}
	return nil, nil
}
