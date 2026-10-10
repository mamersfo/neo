package docker

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContainerNameDeterministic(t *testing.T) {
	a := ContainerName("/tmp/proj")
	b := ContainerName("/tmp/proj")
	if a != b {
		t.Errorf("not deterministic: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "neo-") {
		t.Errorf("missing neo- prefix: %q", a)
	}
}

func TestContainerNameDistinct(t *testing.T) {
	a := ContainerName("/tmp/proj-a")
	b := ContainerName("/tmp/proj-b")
	if a == b {
		t.Errorf("collision: both %q", a)
	}
}

func TestContainerNameValidChars(t *testing.T) {
	n := ContainerName("/some/path with spaces/ü")
	for _, r := range n {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			t.Errorf("invalid char %q in %q", r, n)
		}
	}
}

func TestStatusJSONShape(t *testing.T) {
	st := Status{Name: "neo-abc", Image: "neo4j:5.26", State: "running", Running: true}
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"name", "image", "state", "running"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q in %s", k, b)
		}
	}
	if _, ok := m["bolt_host_port"]; ok {
		t.Errorf("omitempty violated (empty bolt_host_port present): %s", b)
	}
}
