package kubernetes

import (
	"context"
	"fmt"
	"sync"
	"time"

	"chatops-deploy/internal/domain"
	"chatops-deploy/internal/security"
	"chatops-deploy/internal/store/postgres"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Manager struct {
	store   *postgres.Store
	box     *security.SecretBox
	mu      sync.Mutex
	clients map[string]*kubernetes.Clientset
}

func NewManager(store *postgres.Store, box *security.SecretBox) *Manager {
	return &Manager{store: store, box: box, clients: map[string]*kubernetes.Clientset{}}
}
func (m *Manager) Client(ctx context.Context, c domain.Cluster) (*kubernetes.Clientset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%d", c.ID, c.CredentialVersion)
	if client := m.clients[key]; client != nil {
		return client, nil
	}
	plain, err := m.box.Open(c.EncryptedKubeconfig, []byte(fmt.Sprintf("%s:%d", c.ID, c.CredentialVersion)))
	if err != nil {
		return nil, fmt.Errorf("decrypt kubeconfig: %w", err)
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(plain)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	m.clients[key] = client
	return client, nil
}

type RolloutStatus struct {
	Desired, Updated, Available    int32
	Generation, ObservedGeneration int64
	Ready                          bool
	Message                        string
}

func (m *Manager) Status(ctx context.Context, environment domain.AppEnvironment) (RolloutStatus, error) {
	c, err := m.store.GetCluster(ctx, environment.ClusterID)
	if err != nil {
		return RolloutStatus{}, err
	}
	client, err := m.Client(ctx, c)
	if err != nil {
		return RolloutStatus{}, err
	}
	dep, err := client.AppsV1().Deployments(environment.Namespace).Get(ctx, environment.Deployment, metav1.GetOptions{})
	if err != nil {
		return RolloutStatus{}, err
	}
	status := RolloutStatus{Desired: desiredReplicas(dep.Spec.Replicas), Updated: dep.Status.UpdatedReplicas, Available: dep.Status.AvailableReplicas, Generation: dep.Generation, ObservedGeneration: dep.Status.ObservedGeneration}
	status.Ready = status.ObservedGeneration >= dep.Generation && status.Updated == status.Desired && status.Available == status.Desired
	return status, nil
}

func (m *Manager) Deploy(ctx context.Context, environment domain.AppEnvironment, image string, timeout time.Duration) (RolloutStatus, error) {
	c, err := m.store.GetCluster(ctx, environment.ClusterID)
	if err != nil {
		return RolloutStatus{}, err
	}
	client, err := m.Client(ctx, c)
	if err != nil {
		return RolloutStatus{}, err
	}
	var desired int32
	err = retryOnConflict(ctx, func() error {
		dep, err := client.AppsV1().Deployments(environment.Namespace).Get(ctx, environment.Deployment, metav1.GetOptions{})
		if err != nil {
			return err
		}
		desired = desiredReplicas(dep.Spec.Replicas)
		found := false
		for i := range dep.Spec.Template.Spec.Containers {
			if dep.Spec.Template.Spec.Containers[i].Name == environment.Container {
				dep.Spec.Template.Spec.Containers[i].Image = image
				found = true
			}
		}
		if !found {
			return fmt.Errorf("container %s not found", environment.Container)
		}
		_, err = client.AppsV1().Deployments(environment.Namespace).Update(ctx, dep, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return RolloutStatus{}, err
	}
	return m.wait(ctx, client, environment, desired, timeout)
}

func (m *Manager) Rollback(ctx context.Context, environment domain.AppEnvironment, revision int64, timeout time.Duration) (RolloutStatus, error) {
	c, err := m.store.GetCluster(ctx, environment.ClusterID)
	if err != nil {
		return RolloutStatus{}, err
	}
	client, err := m.Client(ctx, c)
	if err != nil {
		return RolloutStatus{}, err
	}
	dep, err := client.AppsV1().Deployments(environment.Namespace).Get(ctx, environment.Deployment, metav1.GetOptions{})
	if err != nil {
		return RolloutStatus{}, err
	}
	selector := metav1.FormatLabelSelector(dep.Spec.Selector)
	sets, err := client.AppsV1().ReplicaSets(environment.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return RolloutStatus{}, err
	}
	var target *appsv1.ReplicaSet
	for i := range sets.Items {
		if sets.Items[i].Annotations["deployment.kubernetes.io/revision"] == fmt.Sprint(revision) {
			target = &sets.Items[i]
			break
		}
	}
	if target == nil {
		return RolloutStatus{}, fmt.Errorf("revision %d not found", revision)
	}
	desired := desiredReplicas(dep.Spec.Replicas)
	err = retryOnConflict(ctx, func() error {
		current, e := client.AppsV1().Deployments(environment.Namespace).Get(ctx, environment.Deployment, metav1.GetOptions{})
		if e != nil {
			return e
		}
		current.Spec.Template = target.Spec.Template
		delete(current.Spec.Template.Labels, "pod-template-hash")
		delete(current.Spec.Template.Annotations, "deployment.kubernetes.io/revision")
		_, e = client.AppsV1().Deployments(environment.Namespace).Update(ctx, current, metav1.UpdateOptions{})
		return e
	})
	if err != nil {
		return RolloutStatus{}, err
	}
	return m.wait(ctx, client, environment, desired, timeout)
}

func (m *Manager) wait(ctx context.Context, client *kubernetes.Clientset, e domain.AppEnvironment, desired int32, timeout time.Duration) (RolloutStatus, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		status, err := deploymentStatus(ctx, client, e, desired)
		if err != nil {
			return RolloutStatus{}, err
		}
		if status.Ready {
			return status, nil
		}
		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-deadline.C:
			return status, fmt.Errorf("rollout timed out: %s", status.Message)
		case <-tick.C:
		}
	}
}
func deploymentStatus(ctx context.Context, client *kubernetes.Clientset, e domain.AppEnvironment, desired int32) (RolloutStatus, error) {
	dep, err := client.AppsV1().Deployments(e.Namespace).Get(ctx, e.Deployment, metav1.GetOptions{})
	if err != nil {
		return RolloutStatus{}, err
	}
	return RolloutStatus{Desired: desired, Updated: dep.Status.UpdatedReplicas, Available: dep.Status.AvailableReplicas, Generation: dep.Generation, ObservedGeneration: dep.Status.ObservedGeneration, Ready: dep.Status.ObservedGeneration >= dep.Generation && dep.Status.UpdatedReplicas == desired && dep.Status.AvailableReplicas == desired}, nil
}
func retryOnConflict(ctx context.Context, fn func() error) error {
	var err error
	for i := 0; i < 5; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !apierrors.IsConflict(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(i+1) * 200 * time.Millisecond):
		}
	}
	return err
}

func desiredReplicas(replicas *int32) int32 {
	if replicas == nil {
		return 1
	}
	return *replicas
}
