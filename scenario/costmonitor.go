package scenario

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	kpricing "github.com/aws/karpenter/pkg/providers/pricing"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

type CostMonitor struct {
	mu                sync.RWMutex
	nodes             map[string]*v1.Node
	nodePrices        map[string]float64
	pods              map[string]*v1.Pod
	log               *os.File
	csv               *csv.Writer
	start             time.Time
	pprov             *kpricing.Provider
	cumulativeCost    float64
	pendingPodSeconds float64
	pendingPods       int
	nodeSelector      *string
	client            *kubernetes.Clientset
}

func NewCostMonitor(ctx context.Context, client *kubernetes.Clientset, name string, nodeSelector *string) (*CostMonitor, error) {
	date := time.Now().Format("2006_01_02_15_04_05")
	logName := fmt.Sprintf("%s-cost-%s.log", name, date)
	f, err := os.Create(logName)
	if err != nil {
		return nil, err
	}

	sess := session.Must(session.NewSessionWithOptions(session.Options{SharedConfigState: session.SharedConfigEnable}))

	papi := kpricing.NewAPI(sess, "us-west-2")
	pprov := kpricing.NewProvider(ctx, papi, ec2.New(sess), "us-west-2")
	if err := pprov.UpdateOnDemandPricing(ctx); err != nil {
		return nil, err
	}
	if err := pprov.UpdateSpotPricing(ctx); err != nil {
		return nil, err
	}
	log.Println("logging to", logName)
	cm := &CostMonitor{
		client:       client,
		nodeSelector: nodeSelector,
		nodes:        map[string]*v1.Node{},
		pods:         map[string]*v1.Pod{},
		nodePrices:   map[string]float64{},
		log:          f,
		csv:          csv.NewWriter(f),
		pprov:        pprov,
	}
	return cm, nil
}

func (c *CostMonitor) Stop() {
	c.csv.Flush()
	c.log.Close()
	log.Printf("total cost: %f", c.cumulativeCost)
}

func (c *CostMonitor) monitorNodes(ctx context.Context) {
	nodeFilterOpts := metav1.ListOptions{}
	if c.nodeSelector != nil {
		nodeFilterOpts.LabelSelector = *c.nodeSelector
	}

	for {
		log.Println("starting node watch")
		nodeWatcher, err := c.client.CoreV1().Nodes().Watch(ctx, nodeFilterOpts)
		if err != nil {
			log.Printf("watching nodes, %s", err)
			continue
		}

	inner:
		for {
			select {
			case ev, ok := <-nodeWatcher.ResultChan():
				if !ok {
					log.Println("restarting node watch")
					break inner
				}
				node, ok := ev.Object.(*v1.Node)
				if ok {
					switch ev.Type {
					case watch.Added:
						c.addNode(node)
					case watch.Modified:
						c.addNode(node)
					case watch.Deleted:
						c.removeNode(node)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}
}

func (c *CostMonitor) monitorPods(ctx context.Context) {

	for {
		log.Println("starting pod watch")
		podWatcher, err := c.client.CoreV1().Pods("default").Watch(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("watching pods, %s", err)
			continue
		}

	inner:
		for {
			select {
			case ev, ok := <-podWatcher.ResultChan():
				if !ok {
					log.Println("restarting pod watch")
					break inner
				}
				pod, ok := ev.Object.(*v1.Pod)
				if ok {
					switch ev.Type {
					case watch.Added:
						c.updatePod(pod)
					case watch.Modified:
						c.updatePod(pod)
					case watch.Deleted:
						c.deletePod(pod)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}
}

// monitorPendingPods monitors pending pods at a higher rate so we can track the pending pod seconds more accurately
func (c *CostMonitor) monitorPendingPods(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			c.mu.RLock()
			c.pendingPods = 0
			for _, p := range c.pods {
				if p.Status.Phase == v1.PodPending {
					c.pendingPods++
				}
			}
			c.pendingPodSeconds += float64(c.pendingPods) * 0.25
			c.mu.RUnlock()
		case <-ctx.Done():
			return
		}
	}
}

func (c *CostMonitor) logCost(ctx context.Context) {
	ticker := time.Tick(1 * time.Second)
	c.csv.Write([]string{
		"time",
		"nodes",
		"per hour cost",
		"cumulative cost",
		"pods",
		"pending pods",
		"pending pod seconds",
	})

	lastReported := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker:
			c.mu.RLock()
			delta := time.Since(c.start)
			perHourCost := 0.0
			for _, n := range c.nodes {
				perHourCost += c.nodePrices[n.Name]
			}

			c.cumulativeCost += perHourCost / 3600.0

			c.csv.Write([]string{
				strconv.FormatFloat(delta.Seconds(), 'g', -1, 64),
				strconv.FormatInt(int64(len(c.nodes)), 10),
				strconv.FormatFloat(perHourCost, 'g', -1, 64),
				strconv.FormatFloat(c.cumulativeCost, 'g', -1, 64),

				strconv.FormatInt(int64(len(c.pods)), 10),
				strconv.FormatInt(int64(c.pendingPods), 10),
				strconv.FormatFloat(c.pendingPodSeconds, 'g', -1, 64),
			})

			if time.Since(lastReported) > time.Minute {
				log.Printf("nodes: %d, per hour cost: %f, cumulative cost: %f pending pod seconds: %f", len(c.nodes), perHourCost, c.cumulativeCost, c.pendingPodSeconds)
				lastReported = time.Now()
			}
			c.csv.Flush()
			c.mu.RUnlock()
		}
	}
}

func (c *CostMonitor) addNode(node *v1.Node) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodes[node.Name] = node
	instanceType := node.Labels[v1.LabelInstanceTypeStable]
	price, ok := c.pprov.OnDemandPrice(instanceType)
	if ok {
		c.nodePrices[node.Name] = price
	} else {
		log.Printf("unable to find node price for %s/%s", node.Name, instanceType)
	}
}

func (c *CostMonitor) removeNode(node *v1.Node) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.nodes, node.Name)
	delete(c.nodePrices, node.Name)
}

func (c *CostMonitor) Start(ctx context.Context, start time.Time) {
	c.start = start
	go c.monitorNodes(ctx)
	go c.monitorPods(ctx)
	go c.monitorPendingPods(ctx)
	go c.logCost(ctx)
}

func (c *CostMonitor) updatePod(pod *v1.Pod) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pods[string(pod.UID)] = pod
}

func (c *CostMonitor) deletePod(pod *v1.Pod) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pods, string(pod.UID))
}
