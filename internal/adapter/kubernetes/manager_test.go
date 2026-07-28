package kubernetes

import (
	"context"
	"testing"

	"chatops-deploy/internal/domain"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDeploymentStatusReportsReadyOnlyWhenObservedAndAvailable(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default", Generation: 3},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr(int32(2)), Template: corev1.PodTemplateSpec{}},
		Status:     appsv1.DeploymentStatus{ObservedGeneration: 3, UpdatedReplicas: 2, AvailableReplicas: 2},
	}
	client := fake.NewSimpleClientset([]runtime.Object{deployment}...)
	environment := domain.AppEnvironment{Namespace: "default", Deployment: "api"}

	status, err := deploymentStatus(context.Background(), client, environment, 2)

	require.NoError(t, err)
	require.True(t, status.Ready)
	require.Equal(t, int32(2), status.Available)
}

func TestDeploymentStatusReportsUnreadyForStaleGeneration(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default", Generation: 3},
		Status:     appsv1.DeploymentStatus{ObservedGeneration: 2, UpdatedReplicas: 2, AvailableReplicas: 2},
	}
	client := fake.NewSimpleClientset(deployment)

	status, err := deploymentStatus(context.Background(), client, domain.AppEnvironment{Namespace: "default", Deployment: "api"}, 2)

	require.NoError(t, err)
	require.False(t, status.Ready)
}

func ptr[T any](value T) *T { return &value }
