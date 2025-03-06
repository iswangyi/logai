package collector

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type KubernetesCollector struct {
	clientset   *kubernetes.Clientset
	logBasePath string
	logCh       chan<- string
	namespace   string
}

func NewKubernetesCollector(kubeconfig string, logBasePath string, namespace string) (*KubernetesCollector, error) {
	var config *rest.Config
	var err error

	if kubeconfig == "" {
		config, err = rest.InClusterConfig()
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &KubernetesCollector{
		clientset:   clientset,
		logBasePath: logBasePath,
		namespace:   namespace,
	}, nil
}

func (k *KubernetesCollector) Start(ctx context.Context) error {
	factory := informers.NewSharedInformerFactory(k.clientset, 30*time.Minute)
	podInformer := factory.Core().V1().Pods().Informer()

	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			k.handlePodLogPaths(pod)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			newPod := newObj.(*corev1.Pod)
			k.handlePodLogPaths(newPod)
		},
	})

	go podInformer.Run(ctx.Done())
	return nil
}

func (k *KubernetesCollector) handlePodLogPaths(pod *corev1.Pod) {
	if pod.Namespace != k.namespace {
		return
	}
	for _, container := range pod.Spec.Containers {
		logPath := filepath.Join(k.logBasePath, "pods", string(pod.UID), container.Name)
		k.logCh <- fmt.Sprintf("%s|%s", logPath, container.Name)
	}
}
