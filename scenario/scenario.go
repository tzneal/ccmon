package scenario

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Scenario struct {
	Name         string         `yaml:"name"`
	Duration     time.Duration  `yaml:"duration"`
	Deployments  []Deployment   `yaml:"deployments"`
	RepeatAfter  *time.Duration `yaml:"repeatAfter"`
	Events       []Event        `yaml:"events"`
	NodeSelector *string        `yaml:"nodeSelector"`
}
type Deployment struct {
	Name   string   `yaml:"name"`
	CPU    Quantity `yaml:"cpu"`
	Memory Quantity `yaml:"memory"`
}

func (d Deployment) K8sName() string {
	return strings.Replace(fmt.Sprintf("ccmon-%s", d.Name), " ", "-", -1)
}

type Event struct {
	Time       time.Duration `yaml:"time"`
	Deployment string        `yaml:"deployment"`
	Replicas   int           `yaml:"replicas"`
	deployment *Deployment
}

func Open(r io.Reader) (*Scenario, error) {
	dec := yaml.NewDecoder(r)
	var s Scenario
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("decoding scenario, %w", err)
	}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("validating scenario, %w", err)
	}
	return &s, nil
}

func (s *Scenario) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("scenario has no name")
	}
	if s.Duration == 0 {
		return fmt.Errorf("scenario has zero length duration")
	}
	if len(s.Events) == 0 {
		return fmt.Errorf("scenario has no events")
	}

	if s.NodeSelector != nil {
		_, err := metav1.ParseToLabelSelector(*s.NodeSelector)
		if err != nil {
			return fmt.Errorf("invalid node selector %q, %w", *s.NodeSelector, err)
		}
	}

	uniqueDeps := map[string]*Deployment{}
	for i := range s.Deployments {
		dep := s.Deployments[i]
		uniqueDeps[dep.Name] = &dep
	}
	if len(uniqueDeps) != len(s.Deployments) {
		return fmt.Errorf("duplicate deployment names")
	}

	for i := range s.Events {
		ev := &s.Events[i]
		dep, ok := uniqueDeps[ev.Deployment]
		if !ok {
			return fmt.Errorf("unknown deployment %s", ev.Deployment)
		}
		ev.deployment = dep
	}

	// do the events repeat?
	if s.RepeatAfter != nil {
		var newEvents []Event
		var nextStart time.Duration
		for _, ev := range s.Events {
			if ev.Time > nextStart {
				nextStart = ev.Time
			}
		}
		nextStart += *s.RepeatAfter

		for nextStart < s.Duration {
			for _, ev := range s.Events {
				cp := ev
				nextStart += cp.Time
				cp.Time = nextStart
				newEvents = append(newEvents, cp)
			}
			nextStart += *s.RepeatAfter
		}
		s.Events = append(s.Events, newEvents...)
	}
	// ensure events are sorted by time
	sort.SliceStable(s.Events, func(i, j int) bool {
		return s.Events[i].Time < s.Events[j].Time
	})
	return nil
}

func (s Scenario) String() string {
	var b bytes.Buffer
	tw := tabwriter.NewWriter(&b, 4, 2, 1, ' ', 0)
	fmt.Fprintf(tw, "Name:\t%s\n", s.Name)
	fmt.Fprintf(tw, "Duration:\t%s\n", s.Duration)

	for _, dep := range s.Deployments {
		fmt.Fprintf(tw, "Deployment:\t%s\n", dep.Name)
		fmt.Fprintf(tw, "\t - CPU:\t%s\n", dep.CPU)
		fmt.Fprintf(tw, "\t - Memory:\t%s\n", dep.Memory)
	}

	fmt.Fprintf(tw, "Events\n")
	for _, ev := range s.Events {
		fmt.Fprintf(tw, " - %s => scale %s to %d\n", ev.Time, ev.Deployment, ev.Replicas)
	}
	tw.Flush()
	return b.String()
}
