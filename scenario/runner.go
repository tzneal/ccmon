package scenario

import (
	"context"
	"fmt"
	"log"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Runner struct {
	client *kubernetes.Clientset
	start  time.Time
}

func NewRunner(kubeConfig string) (*Runner, error) {
	// get our K8s client setup
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client, %w", err)
	}
	return &Runner{
		client: client,
	}, nil
}

func (r *Runner) Execute(ctx context.Context, scen *Scenario) error {
	defer r.cleanup(scen)

	log.Println("creating deployments")
	for _, dep := range scen.Deployments {
		_, err := r.client.AppsV1().Deployments("default").Create(ctx, createDeployment(dep), metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating deployment %s, %w", dep.Name, err)
		}
	}

	cm, err := NewCostMonitor(ctx, r.client, scen.Name, scen.NodeSelector)
	if err != nil {
		return fmt.Errorf("creating cost monitor, %w", err)
	}
	defer cm.Stop()
	r.start = time.Now()
	cm.Start(ctx, r.start)

	r.log("starting scenario")
	for i := range scen.Events {
		ev := scen.Events[i]
		go func() {
			select {
			case <-time.After(ev.Time):
			case <-ctx.Done():
				return
			}
			r.execute(ev)
		}()
	}
	select {
	case <-time.After(scen.Duration):
	case <-ctx.Done():
		r.log("interrupted, exiting")
	}
	return nil
}

func createDeployment(dep Deployment) *appsv1.Deployment {
	replicas := int32(0)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      dep.K8sName(),
			Labels: map[string]string{
				"ccmon": "owned",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": dep.K8sName(),
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      dep.K8sName(),
					Labels: map[string]string{
						"ccmon": "owned",
						"app":   dep.K8sName(),
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "container",
							Image: "public.ecr.aws/eks-distro/kubernetes/pause:3.2",
							Resources: v1.ResourceRequirements{
								Requests: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU:    dep.CPU.Quantity,
									v1.ResourceMemory: dep.Memory.Quantity,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *Runner) execute(ev Event) {
	r.log("scaling %s to %d replicas", ev.Deployment, ev.Replicas)

	for try := 0; try < 100; try++ {
		s, err := r.client.AppsV1().Deployments("default").GetScale(context.Background(), ev.deployment.K8sName(), metav1.GetOptions{})
		if err != nil {
			r.log("unable to get scale for %s, %s", ev.Deployment, err)
			return
		}

		s.Spec.Replicas = int32(ev.Replicas)
		_, err = r.client.AppsV1().Deployments("default").UpdateScale(context.Background(),
			ev.deployment.K8sName(), s, metav1.UpdateOptions{})
		if err == nil {
			break
		}
		if err != nil && try == 99 {
			r.log("unable to scale %s, %s", ev.Deployment, err)
		}
	}
}

func (r *Runner) log(s string, args ...interface{}) {
	line := fmt.Sprintf(s, args...)
	log.Printf("[%s] %s", time.Since(r.start), line)
}

func (r *Runner) cleanup(scen *Scenario) {
	ctx := context.Background()
	gracePeriod := int64(0)
	for _, dep := range scen.Deployments {
		err := r.client.AppsV1().Deployments("default").Delete(ctx, dep.K8sName(),
			metav1.DeleteOptions{
				TypeMeta:           metav1.TypeMeta{},
				GracePeriodSeconds: &gracePeriod,
			})
		if err != nil {
			r.log("deleting deployment %s (%s), %w", dep.Name, dep.K8sName(), err)
		}
	}
}
